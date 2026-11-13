package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/konradk/gotamusique/internal/database"
)

// UploadDir is the folder under the music folder that receives uploads, one
// subfolder per uploader.
const UploadDir = "uploads"

// ErrTooLarge is returned by Store when the upload exceeds the size limit.
var ErrTooLarge = errors.New("file too large")

const (
	maxNameBytes = 200 // longest file or folder name we create
	maxNameTries = 1000
)

// SafeName turns a client-supplied file name into one that is safe to create: a
// bare name with no directory part, no control or reserved characters, not hidden,
// not empty and not too long. The extension is kept. It returns "" when nothing
// usable is left. Only the part after the last slash or backslash is used, so
// "C:\\music\\a.mp3" and "../../a.mp3" both become "a.mp3".
func SafeName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	if name == "." || name == "/" {
		return ""
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 0x20 || r == 0x7f || r == utf8.RuneError:
			return -1
		case strings.ContainsRune(`/\:*?"<>|`, r):
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " .") // no hidden files, no trailing dot or space
	if name == "" {
		return ""
	}
	return truncateName(name, maxNameBytes)
}

// truncateName shortens name to at most max bytes without cutting a character in
// half, keeping the extension (an audio extension decides whether the file is
// accepted at all).
func truncateName(name string, max int) string {
	if len(name) <= max {
		return name
	}
	ext := path.Ext(name)
	if len(ext) > 16 { // not a real extension
		ext = ""
	}
	stem := strings.TrimSuffix(name, ext)
	for len(stem)+len(ext) > max {
		_, size := utf8.DecodeLastRuneInString(stem)
		stem = stem[:len(stem)-size]
	}
	return strings.Trim(stem, " .") + ext
}

// ownerDir is the folder name for an uploader; "user" when nothing usable is left.
func ownerDir(owner string) string {
	if n := SafeName(owner); n != "" {
		return n
	}
	return "user"
}

// Store saves r as a new audio file in uploads/<owner>/ under the music folder and
// adds it to the library. At most maxBytes are accepted (ErrTooLarge beyond that).
//
// The client's file name is sanitised (SafeName) and must carry a known audio
// extension; an existing name is never overwritten, the new file gets " (1)",
// " (2)"... instead. The bytes go to a hidden temporary file first and are renamed
// into place, so a concurrent scan never sees a half-written track, and the file is
// then checked with the same rules as the scan (ffprobe must read it, with an audio
// stream). Anything that fails leaves nothing behind. Problems with the file itself
// are ErrNotIndexable with a message that is safe to show: details, which can
// include server paths, go to the log only.
func (s *Scanner) Store(ctx context.Context, owner, filename string, r io.Reader, maxBytes int64) (*database.MusicRecord, error) {
	root, err := s.resolveRoot()
	if err != nil {
		return nil, err
	}
	name := SafeName(filename)
	if name == "" || !IsAudioFile(name) {
		return nil, fmt.Errorf("%w: only audio files are accepted (%s)", ErrNotIndexable, extensionList())
	}
	user := ownerDir(owner)
	if s.ignDirs[UploadDir] || s.ignDirs[user] {
		return nil, fmt.Errorf("%w: uploads go to a folder the library ignores", ErrNotIndexable)
	}

	dir, err := ensureUploadDir(root, user)
	if err != nil {
		return nil, err
	}
	// Defence in depth: whatever is on disk now, the folder must resolve inside
	// the music folder.
	if real, err := filepath.EvalSymlinks(dir); err != nil || !within(root, real) {
		return nil, fmt.Errorf("%w: the upload folder is not inside the music folder", ErrNotIndexable)
	}

	final, err := reserve(dir, name)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			os.Remove(final) //nolint:errcheck // the reserved placeholder or the finished file
		}
	}()

	tmp, err := os.CreateTemp(dir, ".upload-*.part")
	if err != nil {
		return nil, fmt.Errorf("creating temporary file: %w", err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after a successful rename

	// Read one byte more than allowed to tell "exactly at the limit" from "over it".
	// At the top of the int64 range there is no room for that byte, and no file can
	// be bigger, so the limit is used as it is instead of wrapping around.
	readMax := maxBytes
	if readMax < math.MaxInt64 {
		readMax++
	}
	n, err := io.Copy(tmp, io.LimitReader(ctxReader{ctx, r}, readMax))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, fmt.Errorf("receiving the file: %w", err)
	}
	if n > maxBytes {
		return nil, ErrTooLarge
	}
	if n == 0 {
		return nil, fmt.Errorf("%w: the file is empty", ErrNotIndexable)
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		return nil, fmt.Errorf("saving the file: %w", err)
	}

	rel := UploadDir + "/" + user + "/" + filepath.Base(final)
	rec, err := s.IndexFile(ctx, rel)
	if err != nil {
		if errors.Is(err, ErrNotIndexable) {
			s.log.Warn("upload rejected", slog.String("path", rel), slog.String("err", err.Error()))
			return nil, fmt.Errorf("%w: ffprobe could not read it as an audio file", ErrNotIndexable)
		}
		return nil, err
	}
	keep = true
	return rec, nil
}

// ensureUploadDir returns <root>/uploads/<user>, creating what is missing one level
// at a time. Each level that already exists must be a real directory: a symlink
// (or a file) is refused before anything is created below it. os.MkdirAll must not
// be used here, because it follows symlinks, so a symlinked uploads/ would get its
// <user> folder created in the link's target before any check could run. Folders
// that are symlinks would not be scanned either (the scan does not follow them), so
// their files would vanish from the library at the next scan.
func ensureUploadDir(root, user string) (string, error) {
	dir := root
	for _, part := range []string{UploadDir, user} {
		dir = filepath.Join(dir, part)
		fi, err := os.Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			// dir's parent is root or a real directory checked just above.
			if err := os.Mkdir(dir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
				return "", fmt.Errorf("creating upload folder: %w", err)
			}
			fi, err = os.Lstat(dir)
		}
		if err != nil {
			return "", fmt.Errorf("checking upload folder: %w", err)
		}
		if !fi.IsDir() { // Lstat: a symlink is not a directory here
			return "", fmt.Errorf("%w: %s is not a plain folder (a symlink?)", ErrNotIndexable, part)
		}
	}
	return dir, nil
}

// reserve creates an empty file for name in dir, or for the first free
// "name (1).ext", "name (2).ext"... The O_EXCL create is what makes the name ours:
// two uploads of the same name at once cannot both get it, and an existing file is
// never touched.
func reserve(dir, name string) (string, error) {
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 0; i < maxNameTries; i++ {
		candidate := name
		if i > 0 {
			candidate = truncateName(fmt.Sprintf("%s (%d)%s", stem, i, ext), maxNameBytes)
		}
		p := filepath.Join(dir, candidate)
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			f.Close()
			return p, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("creating %q: %w", candidate, err)
		}
	}
	return "", fmt.Errorf("%w: too many files with that name", ErrNotIndexable)
}

func extensionList() string {
	exts := make([]string, 0, len(audioExts))
	for e := range audioExts {
		exts = append(exts, e)
	}
	slices.Sort(exts)
	return strings.Join(exts, " ")
}

// ctxReader stops a copy when ctx ends, so a stalled client cannot hold an upload
// open past its deadline.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

# 2-02 — Local File Media

**Status:** todo  
**Depends on:** 2-01  
**Unlocks:** 2-05

## Objective

Implement the `file` media item type: validate a path, extract audio metadata via ffprobe, and play through the existing audio pipeline.

## FileItem

```go
type FileItem struct {
    ID       string   // sha1(path) hex
    Path     string   // relative to music_folder
    Title    string
    Artist   string
    Duration time.Duration
    Thumb    string   // base64 PNG or ""
    Tags     []string
}

func (f *FileItem) Validate() error  // check file exists and is readable
func (f *FileItem) Prepare() error   // no-op (file is local)
func (f *FileItem) StreamURL() string // absolute path
```

## Metadata extraction

```
ffprobe -v quiet -print_format json -show_format -show_streams <path>
```

Parse: `format.tags.title`, `format.tags.artist`, `format.duration`.  
Embedded album art: extract with `ffmpeg -i <path> -an -vcodec copy <tmp>.png`, base64-encode, delete tmp.

## Deliverables

- `internal/media/file/item.go`
- `internal/media/file/metadata.go` — ffprobe wrapper
- `internal/media/file/id.go` — `IDFromPath(path string) string`
- Tests: fixture audio file (generate a 1-second silent WAV with ffmpeg in TestMain)

## Acceptance criteria

- `!file song.mp3` adds the file and plays it
- Title and artist extracted from ID3 tags
- Missing file returns validation error and is skipped with a channel message

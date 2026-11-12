package command

import (
	"fmt"

	"layeh.com/gumble/gumble"
)

// Ctx is everything a command handler needs to know about who called it and where
// its output goes. Chat builds one from a Mumble text message; other front ends (the
// HTTP API) build their own, so handlers never touch gumble messages directly.
type Ctx struct {
	// User is the caller's name; used for admin checks.
	User string
	// ChannelID identifies the conversation, so per-channel state (the shortlist, the
	// radio search cache) stays separate. 0 when there is none.
	ChannelID uint32
	// Out receives the command's output. Nil discards it.
	Out func(text string)
	// Private delivers text to the caller alone (a Mumble private message). Nil
	// when there is no such channel; commands that hand out secrets must refuse
	// to run without it rather than fall back to Out. It returns an error when the
	// send itself failed (the user disconnected). gumble reports nothing about a
	// message the server drops afterwards, so a nil error is not a receipt.
	Private func(text string) error
	// Sender is the Mumble user who sent the message, when there is one. Only
	// JoinTarget uses it.
	Sender *gumble.User
}

// Send writes text to the caller. It is safe to call with no Out.
func (c *Ctx) Send(text string) {
	if c != nil && c.Out != nil {
		c.Out(text)
	}
}

// JoinTarget is the channel the caller is in, or nil when unknown (!joinme).
func (c *Ctx) JoinTarget() *gumble.Channel {
	if c == nil || c.Sender == nil {
		return nil
	}
	return c.Sender.Channel
}

// chatCtx builds the context for a Mumble text message. Replies go to the message's
// first channel; the send recovers from the gumble panics that happen when the client
// disconnects between receiving the message and answering it.
func chatCtx(msg *gumble.TextMessage) *Ctx {
	c := &Ctx{Sender: msg.Sender}
	if msg.Sender != nil {
		c.User = msg.Sender.Name
	}
	if msg.Sender != nil {
		sender := msg.Sender
		c.Private = func(text string) (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("private message not sent: %v", r)
				}
			}()
			sender.Send(text)
			return nil
		}
	}
	if len(msg.Channels) > 0 && msg.Channels[0] != nil {
		ch := msg.Channels[0]
		c.ChannelID = ch.ID
		c.Out = func(text string) {
			defer func() { recover() }() //nolint:errcheck
			ch.Send(text, false)
		}
	}
	return c
}

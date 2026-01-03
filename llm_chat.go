package ion

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

type LLMChat struct {
	ID          string
	Name        string
	Prompt      string
	Completion  LLM
	Messages    []Message
	Uncommitted []Message
	Muted       bool
	Persistent  bool
	Meta        Meta
}

func ReadLLMChat(id, name string) (*LLMChat, error) {
	c := LLMChat{ID: id, Name: name}
	if err := c.read(); err != nil {
		return nil, ErrChat.Wrap(err)
	}
	return &c, nil
}

func NewLLMChat(prompt string, t ...Tool) *LLMChat {
	return &LLMChat{
		ID:     UUID(),
		Prompt: prompt,
		Completion: LLM{
			Tool: t,
		},
	}
}

func (c *LLMChat) Complete(ctx context.Context, m ...Message) ([]Message, error) {
	if c.ID == "" {
		c.ID = UUID()
		Metrics.Count("chats{name=%q}", 1, c.Name)
	}
	if c.Meta == nil {
		c.Meta = Meta{}
	}
	defer func() { c.Uncommitted = nil }()
	if len(c.Messages) == 0 && c.ID != "" {
		if err := c.read(); err != nil {
			return nil, ErrChat.Wrap(err)
		}
		if len(c.Messages) == 0 {
			c.Uncommitted = append([]Message{
				{Role: "system", Content: "chatID: " + c.ID},
				{Role: "system", Content: "chat initial data" + c.Meta.String()},
				{Role: "system", Content: c.Prompt},
			}, c.Uncommitted...)
		}
	}
	if len(c.Messages) > 0 {
		c.Uncommitted = append(c.Messages, c.Uncommitted...)
	}
	if len(m) > 0 {
		c.Uncommitted = append(c.Uncommitted, m...)
	}

	if c.Muted {
		c.Messages = c.Uncommitted
		return nil, c.store()
	}
	o, err := c.Completion.Response(ctx, c.Uncommitted...)
	if err != nil {
		return nil, err
	}
	c.Messages = o
	if err := c.store(); err != nil {
		return nil, ErrChat.Wrap(err)
	}
	return c.Messages, nil
}

func (c *LLMChat) Read(message string) (string, error) {
	var m []Message
	if message != "" {
		m = append(m, Message{Role: "user", Content: message})
	}
	o, err := c.Complete(ctx, m...)
	if err != nil {
		return "", err
	}
	if n := len(o); n >= 1 {
		return o[n-1].Content, nil
	}
	return "", nil
}

func (c *LLMChat) Mute(b bool) (*LLMChat, error) {
	c.Muted = b
	return c, c.store()
}

func (c *LLMChat) Talk() error {
	fmt.Println("-----------------------------CHAT-----------------------------")
	if c.ID != "" && len(c.Messages) == 0 {
		if err := c.read(); err != nil {
			return ErrChat.Wrap(err)
		}
	}
	for _, m := range c.Messages {
		if m.Role == "user" {
			fmt.Printf("> %s\n", m.Content)
		} else {
			fmt.Printf("# %s\n", m.Content)
		}
	}
	keyboard := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		txt, _ := keyboard.ReadString('\n')
		txt = strings.TrimSpace(txt)
		if txt == "bye" {
			return nil
		}
		res, err := c.Read(txt)
		if err != nil {
			return err
		}
		fmt.Printf("# %s\n", res)
	}
}

func (c *LLMChat) Put(text string) *LLMChat {
	c.Uncommitted = append(c.Uncommitted, Message{Role: "user", Content: text})
	return c
}

func (c *LLMChat) String() string {
	s := "id: " + c.ID + "\n"
	for _, m := range c.Messages {
		s += fmt.Sprintf("--------------\n%s\n%s\n\n", m.Role, m.Content)
	}
	return s
}

func (c *LLMChat) IsEmpty() bool {
	return len(c.Messages) == 0 && len(c.Uncommitted) == 0
}

func (c *LLMChat) Answer() string {
	if len(c.Messages) == 0 {
		return ""
	}
	return c.Messages[len(c.Messages)-1].Content
}

func (c *LLMChat) read() error {
	if c.ID == "" {
		return Errorf("id not found")
	}
	if Get(ctx, c.key(), c) < 0 {
		return Errorf("read")
	}
	return nil
}

func (c *LLMChat) key() string {
	n := "default"
	if c.Name != "" {
		n = c.Name
	}
	return fmt.Sprintf("ai:chat:%s:%s", n, c.ID)
}

func (c *LLMChat) store() error {
	if !c.Persistent {
		return nil
	}
	if c.ID == "" {
		return Errorf("id not found")
	}
	Set(ctx, c.key(), c)
	return nil
}

var ErrChat = ErrAI.New("chat")

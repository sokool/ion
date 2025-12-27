package ion

import (
	"bytes"
	"encoding/json"
	"text/template"
	"time"
)

type Prompt string

func (p Prompt) Name(s string) Prompt {
	r := ParseLLMChat(p)
	r.Name = s
	return r.Convert()
}

func (p Prompt) Cache(d time.Duration) Prompt {
	r := ParseLLMChat(p)
	r.Completion.Cache = d
	return r.Convert()
}

func (p Prompt) Model(name string) Prompt {
	r := ParseLLMChat(p)
	r.Completion.Model = name
	return r.Convert()
}

func (p Prompt) Option(name string, value any) Prompt {
	r := ParseLLMChat(p)
	r.Completion = *r.Completion.Option(name, value)
	return r.Convert()
}

func (p Prompt) Temperature(t float64) Prompt {
	r := ParseLLMChat(p)
	r.Completion.Temperature = t
	return r.Convert()
}

func (p Prompt) Completion() LLMCompletion {
	return ParseLLMChat(p).Completion
}

func (p Prompt) Chat() *LLMChat {
	return ParseLLMChat(p).LLMChat
}

func (p Prompt) Params(j Meta) Prompt {
	c := ParseLLMChat(p)
	c.Meta = j
	return c.Convert()
}

func (p Prompt) Param(key string, value any) Prompt {
	r := ParseLLMChat(p)
	r.Meta[key] = value
	return r.Convert()
}

func (p Prompt) parse(s string, j Meta) (string, error) {
	t, err := template.
		New("parser").
		Option("missingkey=zero").
		Delims("{", "}").
		Parse(s)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, j); err != nil {
		return "", err
	}
	return b.String(), nil
}

func (p Prompt) Ask(text ...string) (Prompt, error) {
	r := ParseLLMChat(p)
	s, err := p.parse(r.Prompt, r.Meta)
	if err != nil {
		return "", err
	}
	if r.IsEmpty() && len(text) == 0 {
		text = append(text, "generate a response based on already given messages in this context")
	}
	for i := range text {
		r.Put(text[i])
	}
	r.ID, r.Prompt = UUID(s), s
	if _, err := r.Complete(ctx); err != nil {
		return "", err
	}
	return r.Convert(), nil
}

func (p Prompt) Message(text ...string) (string, error) {
	r := ParseLLMChat(p)
	if r.IsEmpty() {
		n, err := p.Ask(text...)
		if err != nil {
			return "", err
		}
		r = ParseLLMChat(n)
	}
	return r.Answer(), nil
}

type prompt struct {
	*LLMChat
}

func ParseLLMChat(p Prompt) *prompt {
	r := prompt{&LLMChat{Prompt: string(p), Meta: Meta{}}}
	if err := json.Unmarshal([]byte(p), &r); err != nil {
		//
	}
	return &r
}

func (p *prompt) String() string {
	b, _ := json.Marshal(p)
	return string(b)
}

func (p *prompt) Convert() Prompt {
	return Prompt(p.String())
}

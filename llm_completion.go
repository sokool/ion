package ion

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// LLMCompletion represents a configuration structure for generating text via an LLM API.

type LLMCompletion struct {
	// Model specifies the language model to use for text generation.
	Model string
	// Cache defines the cache duration, it will store response in local storage (memory/redis/etc)
	// see 	UseStore function.
	Cache time.Duration
	// Temperature controls the randomness of the generated output.
	Temperature float64
	// Tools provides functions and schemas for tool-based completions.
	Tool []LLMTool

	Options JSON
}

func (c *LLMCompletion) Complete(ctx context.Context, m ...Message) ([]Message, error) {
	api, vendor, err := c.api()
	if err != nil {
		return nil, ErrCompletion.Wrap(err)
	}
	switch vendor {
	case "ChatGPT":
		return c.chatGPT(ctx, api, m...)
	case "Gemini":
		return c.gemini(ctx, api, m...)
	default:
		return c.chatGPT(ctx, api, m...)
	}

}

func (c *LLMCompletion) Read(message string) (string, error) {
	m, err := c.Complete(ctx, Message{Role: "user", Content: message})
	if err != nil {
		return "", err
	}
	if n := len(m); n >= 1 {
		return m[n-1].Content, nil
	}
	return "", nil
}

func (c *LLMCompletion) MDTool(markdown string, fn Function) error {
	t, err := NewLLMToolMD(markdown, fn)
	if err != nil {
		return err
	}
	c.Tool = append(c.Tool, t)
	return nil
}

func (c *LLMCompletion) Option(name string, value any) *LLMCompletion {
	if c.Options == nil {
		c.Options = JSON{}
	}
	c.Options[name] = value
	return c
}

func (c *LLMCompletion) gemini(ctx context.Context, api *API, m ...Message) ([]Message, error) {
	var tools []JSON
	for i := range c.Tool {
		tools = append(tools, c.Tool[i].Schemas...)
	}

	var sys, cts, tls []JSON
	for i := range m {
		rol, txt := m[i].Role, m[i].Content
		switch rol {
		case "system":
			sys = append(sys, JSON{
				"text": txt,
			})
		case "assistant":
			cts = append(cts, JSON{
				"role": "model",
				"parts": []JSON{
					{"text": txt},
				},
			})
		case "function":
			cts = append(cts, JSON{
				"role": "user",
				"parts": []JSON{
					{
						"functionResponse": JSON{
							"name":     m[i].Name,
							"response": JSON{"result": txt},
						},
					},
				},
			})
		case "user":
			cts = append(cts, JSON{
				"role": "user",
				"parts": []JSON{
					{"text": txt},
				},
			})
		}
	}
	var fns []JSON
	for _, t := range c.Tool {
		for _, s := range t.Schemas {
			fns = append(fns, s.
				Select("function").
				Delete("parameters.additionalProperties"),
			)
		}
	}

	if len(fns) > 0 {
		tls = append(tls, JSON{"functionDeclarations": fns})
	}
	if _, ok := c.Options["google_search"]; ok {
		tls = append(tls, JSON{"google_search": JSON{}})
	}
	req := JSON{}
	if len(sys) != 0 {
		req["system_instruction"] = JSON{"parts": sys}
	}
	if len(cts) != 0 {
		req["contents"] = cts
	}
	if len(tls) != 0 {
		req["tools"] = tls
	}
	res, err := api.
		Endpoint("/v1beta/models/%s:generateContent", c.Model).
		Context(ctx).
		Cache(c.Cache).
		Post(req)
	if err != nil {
		return nil, ErrCompletion.Wrap(err)
	}
	for cds := range res.Select("candidates").Each {
		rol := cds.Text("content.role")
		switch rol {
		case "model":
			rol = "assistant"
		}
		for p := range cds.Select("content.parts").Each {
			fnn := p.Text("functionCall.name")
			fna := p.Select("functionCall.args")
			txt := p.Text("text")
			if m, err = c.tool(ctx, m, "", fnn, fna); err != nil {
				return nil, err
			}
			if txt != "" {
				m = append(m, Message{Role: rol, Content: txt})
			}
		}
	}

	return m, nil
}

func (c *LLMCompletion) chatGPT(ctx context.Context, api *API, msg ...Message) ([]Message, error) {
	var tools []JSON
	for i := range c.Tool {
		tools = append(tools, c.Tool[i].Schemas...)
	}

	var mm []JSON
	for _, m := range msg {
		if _, ok := m.Meta["tool_calls"]; ok && m.Content == "" {
			mm = append(mm, JSON{
				"role": "assistant",
				"tool_calls": []JSON{
					m.Meta.Select("tool_calls"),
				},
			})
			continue
		}

		y := JSON{"role": m.Role, "content": m.Content, "userType": m.UserType}
		if m.Role == "function" {
			y["role"], y["tool_call_id"] = "tool", m.ID
		}
		mm = append(mm, y)
	}
	res, err := api.Endpoint("/v1/chat/completions").Context(ctx).Cache(c.Cache).Post(JSON{
		"model":       c.Model,
		"tools":       tools,
		"temperature": c.Temperature,
		"messages":    mm,
	})
	if err != nil {
		return nil, ErrCompletion.Wrap(err)
	}
	if s := res.Text("choices[0].message.content"); s != "" {
		msg = append(msg, Message{Role: res.Text("choices[0].message.role"), Content: s})
	}

	for fcs := range res.Select("choices[0].message.tool_calls").Each {
		fid := fcs.Text("id")
		fnn := fcs.Text("function.name")
		fna, err := NewJSON(fcs.Bytes("function.arguments"))
		if err != nil {
			return nil, ErrCompletion.Wrap(err)
		}
		fna["_method"], fna["_methodID"] = fnn, fid
		msg = append(msg, Message{Role: "assistant", UserType: "llm", Meta: JSON{"tool_calls": fcs}})
		if msg, err = c.tool(ctx, msg, fid, fnn, fna); err != nil {
			return nil, err
		}
	}

	return msg, nil
}

func (c *LLMCompletion) api() (*API, string, error) {
	vendor := "ChatGPT"
	if strings.HasPrefix(c.Model, "gemini") {
		vendor = "Gemini"
	}
	api, err := NewAPI(fmt.Sprintf("%s_URL", strings.ToTitle(vendor)))
	if err != nil {
		return nil, vendor, err
	}
	def := JSON{}
	if c.Model == "" {
		c.Model = api.URL.Query("model")
		def["model"] = c.Model

	}
	if strings.HasPrefix(c.Model, "gpt-5") && c.Temperature != 1 {
		c.Temperature = 1
		def["temperature"] = c.Temperature
	}
	if !def.IsEmpty() {
		log_.Debugf(vendor+":default config applied %v", def)
	}
	api.Name = vendor
	return api, vendor, nil
}

func (c *LLMCompletion) tool(ctx context.Context, msg []Message, id, name string, data JSON) ([]Message, error) {
	if name == "" {
		return msg, nil
	}
	for i := range c.Tool {
		if !c.Tool[i].HasName(name) {
			continue
		}
		res, dispatch := c.Tool[i].Execute(data)
		msg = append(msg, Message{ID: id, Name: name, Role: "function", Content: res})
		// If a tool responds and the result is dispatchable, call the LLM again.
		if dispatch {
			return c.Complete(ctx, msg...)
		}
	}
	return msg, nil
}

type Message struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Role     string `json:"role"`
	UserType string `json:"userType,omitempty"`
	Content  string `json:"content"`
	Meta     JSON   `json:"meta"`
}

var (
	ErrAI         = Errorf("ai")
	ErrCompletion = ErrAI.New("completion")
)

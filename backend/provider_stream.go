package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

type invalidDraftError struct{}

func (*invalidDraftError) Error() string {
	return "文案未完整生成或格式不符合四页要求，调用已计费"
}

type textStream struct {
	Text, ID, Finish string
	Input, Output    int64
}

// Require both an explicit terminal marker and a complete usage record. EOF alone
// is not proof of completion and must never release an ambiguous reservation.
func readTextStream(r io.Reader, emit func(string)) (textStream, error) {
	var out textStream
	var content strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	var data []string
	var pending strings.Builder
	last := time.Now()
	done, usage := false, false
	consume := func() error {
		if len(data) == 0 {
			return nil
		}
		s := strings.Join(data, "\n")
		data = nil
		if s == "[DONE]" {
			done = true
			return nil
		}
		var v struct {
			ID      string          `json:"id"`
			Error   json.RawMessage `json:"error"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				Finish string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				Input  *int64 `json:"prompt_tokens"`
				Output *int64 `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(s), &v) != nil || (len(v.Error) > 0 && string(v.Error) != "null") {
			return errors.New("上游流式响应异常，保留额度待核对")
		}
		if v.ID != "" {
			out.ID = v.ID
		}
		if v.Usage != nil {
			if v.Usage.Input == nil || v.Usage.Output == nil || *v.Usage.Input < 0 || *v.Usage.Output < 0 || *v.Usage.Input > 2_000_000 || *v.Usage.Output > 2_000_000 {
				return errors.New("上游用量无效，保留额度待核对")
			}
			out.Input, out.Output, usage = *v.Usage.Input, *v.Usage.Output, true
		}
		for _, c := range v.Choices {
			content.WriteString(c.Delta.Content)
			pending.WriteString(c.Delta.Content)
			if c.Finish != "" {
				out.Finish = c.Finish
			}
		}
		if content.Len() > 1<<20 {
			return errors.New("响应超出限制，待核对")
		}
		if time.Since(last) > 300*time.Millisecond && pending.Len() > 0 {
			emit(pending.String())
			pending.Reset()
			last = time.Now()
		}
		return nil
	}
	var err error
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			err = consume()
			if err != nil || done {
				break
			}
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			// Bound accumulated multiline SSE data as well as each individual line.
			if len(strings.Join(data, "\n")) > 2<<20 {
				err = errors.New("流式事件过大，待核对")
				break
			}
		}
	}
	if err == nil && !done {
		err = consume()
	}
	out.Text = content.String()
	if pending.Len() > 0 {
		emit(pending.String())
	}
	if err != nil {
		return out, err
	}
	if scanner.Err() != nil || !done || !usage || out.Finish == "" {
		return out, errors.New("流式响应未完整结束或缺少用量，保留预留额待核对")
	}
	return out, nil
}

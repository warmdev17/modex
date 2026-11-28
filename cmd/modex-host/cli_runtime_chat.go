package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
)

func cliSafeText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
}

type cliDecision struct {
	ID      string `json:"decision_id"`
	Method  string `json:"method"`
	Thread  string `json:"thread_id"`
	Context struct {
		Reason    string   `json:"reason"`
		Command   string   `json:"command"`
		Choices   []string `json:"availableDecisions"`
		Questions []struct {
			ID       string `json:"id"`
			Question string `json:"question"`
			Options  []struct {
				Label       string `json:"label"`
				Description string `json:"description"`
			} `json:"options"`
		} `json:"questions"`
	} `json:"context"`
}

func cliDecisionExtra(raw json.RawMessage) []string {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil
	}
	var keys []string
	for key := range fields {
		switch key {
		case "command", "reason", "availableDecisions", "questions":
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var lines []string
	for _, key := range keys {
		var value string
		if json.Unmarshal(fields[key], &value) != nil {
			value = string(fields[key])
		}
		lines = append(lines, cliSafeText(key+": "+value))
	}
	return lines
}

type cliChatReply struct {
	kind string
	data json.RawMessage
	err  error
}
type cliChatLine struct {
	text string
	err  error
}

type cliChatEntry struct {
	label string
	text  string
}

type cliChatOutput struct {
	start func()
	line  func(cliChatEntry)
	delta func(text string, begin bool)
}

func runAgentCLIChat(ctx context.Context, input *bufio.Reader, out io.Writer, client *localCLIClient, agentID, agentLabel, workspace, threadID string) error {
	return runAgentCLIChatWithOutput(ctx, input, client, agentID, workspace, threadID, cliChatOutput{
		start: func() {
			cliRenderFrame(out, cliSafeText(agentLabel), cliSafeText(workspace), nil, "Type a message · /older · /stop · /quit")
		},
		line: func(entry cliChatEntry) {
			if entry.label != "" {
				fmt.Fprintln(out, entry.label+": "+entry.text)
			} else {
				fmt.Fprintln(out, entry.text)
			}
		},
		delta: func(text string, begin bool) {
			if begin {
				fmt.Fprint(out, "assistant: ")
			}
			fmt.Fprint(out, text)
		},
	})
}

func runAgentCLIChatWithOutput(ctx context.Context, input *bufio.Reader, client *localCLIClient, agentID, workspace, threadID string, out cliChatOutput) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	client.watchEvents()
	params := map[string]any{"workspace": workspace}
	method := "threads/resume"
	if threadID == "" {
		method = "threads/start"
		params["agent"] = agentID
	} else {
		params["thread_id"] = threadID
	}
	result, err := client.Call(ctx, method, params)
	if err != nil {
		return err
	}
	var initial struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if json.Unmarshal(result, &initial) != nil || initial.Thread.ID == "" {
		return errors.New("invalid thread response")
	}
	threadID = initial.Thread.ID
	result, err = client.Call(ctx, "threads/read", map[string]any{"thread_id": threadID, "workspace": workspace, "include_turns": true, "limit": 30})
	if err != nil {
		return err
	}
	historySequence := client.historySequence()
	var history struct {
		Messages []struct {
			ID   string `json:"id"`
			Role string `json:"role"`
			Text string `json:"text"`
		} `json:"messages"`
		Active   bool    `json:"active_turn"`
		ActiveID string  `json:"active_turn_id"`
		Next     *string `json:"next_cursor"`
	}
	if json.Unmarshal(result, &history) != nil {
		return errors.New("invalid history response")
	}
	var lines []cliChatEntry
	addLine := func(text string) { lines = append(lines, cliChatEntry{text: text}) }
	addMessage := func(label, text string) { lines = append(lines, cliChatEntry{label, cliSafeText(text)}) }
	historyUsers := map[string]bool{}
	historyAssistant := map[string]string{}
	for _, m := range history.Messages {
		if m.ID != "" {
			if m.Role == "user" {
				historyUsers[m.ID] = true
			}
			if m.Role == "assistant" {
				historyAssistant[m.ID] = m.Text
			}
		}
		if (m.Role == "user" || m.Role == "assistant") && m.Text != "" {
			addMessage(m.Role, m.Text)
		}
	}
	if history.Next != nil && *history.Next != "" {
		addLine("Use /older to load earlier messages.")
	}
	busy := history.ActiveID
	activeUnknown := history.Active && busy == ""
	completedSinceStart := false
	if history.Active && busy == "" {
		addLine("A turn is active; wait for its status.")
	}
	out.start()
	streamOpen := false
	draw := func() {
		if len(lines) != 0 && streamOpen {
			out.line(cliChatEntry{})
			streamOpen = false
		}
		for _, line := range lines {
			out.line(line)
		}
		clear(lines)
		lines = lines[:0]
	}
	writeDelta := func(text string) {
		out.delta(cliSafeText(text), !streamOpen)
		streamOpen = true
	}
	draw()
	inputs := make(chan cliChatLine)
	ack := make(chan struct{})
	go func() {
		for {
			line, e := input.ReadString('\n')
			select {
			case inputs <- cliChatLine{strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), e}:
			case <-ctx.Done():
				return
			}
			if e != nil {
				return
			}
			select {
			case <-ack:
			case <-ctx.Done():
				return
			}
		}
	}()
	replies := make(chan cliChatReply, 8)
	call := func(kind, method string, p any) {
		go func() {
			data, e := client.Call(ctx, method, p)
			select {
			case replies <- cliChatReply{kind, data, e}:
			case <-ctx.Done():
			}
		}()
	}
	answers := map[string]any{}
	question := 0
	var decision *cliDecision
	var queue []*cliDecision
	contexts := map[string]json.RawMessage{}
	known := map[string]bool{}
	showNext := func() {
		if decision != nil || len(queue) == 0 {
			return
		}
		decision = queue[0]
		queue = queue[1:]
		d := decision
		question = 0
		answers = map[string]any{}
		addLine("Permission/input required: " + cliSafeText(d.Context.Reason))
		if d.Context.Command != "" {
			addLine(cliSafeText(d.Context.Command))
		}
		for _, line := range cliDecisionExtra(contexts[d.ID]) {
			addLine(line)
		}
		if d.Method == "item/fileChange/requestApproval" && len(d.Context.Choices) == 0 {
			d.Context.Choices = []string{"decline", "cancel"}
		}
		if len(d.Context.Questions) > 0 {
			addLine(cliSafeText(d.Context.Questions[0].Question))
			for _, o := range d.Context.Questions[0].Options {
				addLine(cliSafeText("  " + o.Label + " " + o.Description))
			}
		} else {
			var choices []string
			for _, choice := range d.Context.Choices {
				if choice == "accept" || choice == "decline" || choice == "cancel" {
					choices = append(choices, choice)
				}
			}
			addLine("Choose: " + cliSafeText(strings.Join(choices, ", ")))
		}
	}
	pending := false
	olderCursor := ""
	streamed := map[string]*strings.Builder{}
	for id, value := range historyAssistant {
		streamed[id] = new(strings.Builder)
		streamed[id].WriteString(value)
	}
	overlap := map[string]int{}
	needAck := false
	for {
		if needAck {
			select {
			case ack <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}
			needAck = false
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-client.done:
			return errors.New("supervisor disconnected; reconnect to refresh history")
		case r := <-replies:
			if r.kind == "turn" {
				pending = false
			}
			if r.err != nil {
				if r.kind == "history" && olderCursor != "" {
					history.Next = &olderCursor
				}
				addLine("Error: " + cliSafeText(r.err.Error()))
				draw()
				continue
			}
			if r.kind == "history" {
				var page struct {
					Messages []struct {
						Role string `json:"role"`
						Text string `json:"text"`
					} `json:"messages"`
					Next *string `json:"next_cursor"`
				}
				if json.Unmarshal(r.data, &page) != nil {
					addLine("Invalid history response.")
				} else {
					history.Next = page.Next
					olderCursor = ""
					addLine("Earlier messages:")
					for _, m := range page.Messages {
						if (m.Role == "user" || m.Role == "assistant") && m.Text != "" {
							addMessage(m.Role, m.Text)
						}
					}
				}
				draw()
				continue
			}
			if r.kind == "turn" {
				var v struct {
					Turn struct {
						ID string `json:"id"`
					} `json:"turn"`
				}
				if json.Unmarshal(r.data, &v) == nil && busy == "" && !completedSinceStart {
					busy = v.Turn.ID
				}
			}
		case ev := <-client.events:
			var p struct {
				ThreadID string `json:"threadId"`
				Thread   string `json:"thread_id"`
				TurnID   string `json:"turnId"`
				ItemID   string `json:"itemId"`
				Delta    string `json:"delta"`
				Turn     struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"turn"`
				Item struct {
					ID      string `json:"id"`
					Text    string `json:"text"`
					Type    string `json:"type"`
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"item"`
				DecisionID string `json:"decision_id"`
			}
			if json.Unmarshal(ev.Params, &p) != nil {
				continue
			}
			tid := p.ThreadID
			if tid == "" {
				tid = p.Thread
			}
			if ev.Event != "decision/resolved" && ev.Event != "decision/reset" && tid != threadID {
				continue
			}
			switch ev.Event {
			case "turn/started":
				activeUnknown = false
				completedSinceStart = false
				busy = p.Turn.ID
				if busy == "" {
					busy = p.TurnID
				}
				addLine("Turn started. Use /stop to interrupt.")
			case "turn/completed":
				activeUnknown = false
				completedSinceStart = true
				busy = ""
				pending = false
				status := p.Turn.Status
				switch status {
				case "failed":
					addLine("Turn failed.")
				case "interrupted":
					addLine("Turn interrupted.")
				default:
					addLine("Turn completed.")
				}
				clear(streamed)
			case "item/agentMessage/delta":
				delta := p.Delta
				if ev.seq <= historySequence && p.ItemID != "" {
					historyText := historyAssistant[p.ItemID]
					used := overlap[p.ItemID]
					if used < len(historyText) && strings.HasPrefix(historyText[used:], delta) {
						overlap[p.ItemID] += len(delta)
						delta = ""
					} else if used < len(historyText) && strings.HasPrefix(delta, historyText[used:]) {
						delta = delta[len(historyText)-used:]
						overlap[p.ItemID] = len(historyText)
					} else {
						overlap[p.ItemID] = len(historyText)
					}
				}
				if delta != "" {
					writeDelta(delta)
					if streamed[p.ItemID] == nil {
						streamed[p.ItemID] = new(strings.Builder)
					}
					streamed[p.ItemID].WriteString(delta)
				}

			case "item/completed":
				if p.Item.Type == "agentMessage" && p.Item.Text != "" {
					previous := ""
					if b := streamed[p.Item.ID]; b != nil {
						previous = b.String()
					}
					if strings.HasPrefix(p.Item.Text, previous) {
						suffix := p.Item.Text[len(previous):]
						if suffix != "" {
							writeDelta(suffix)
						}
					} else {
						addMessage("assistant (updated)", p.Item.Text)
					}
				}
				if p.Item.Type == "userMessage" && !(ev.seq <= historySequence && p.Item.ID != "" && historyUsers[p.Item.ID]) {
					for _, v := range p.Item.Content {
						if v.Text != "" {
							addMessage("user", v.Text)
						}
					}
				}
			case "decision/required":
				var d cliDecision
				if json.Unmarshal(ev.Params, &d) != nil || d.ID == "" || known[d.ID] {
					continue
				}
				known[d.ID] = true
				var raw struct {
					Context json.RawMessage `json:"context"`
				}
				_ = json.Unmarshal(ev.Params, &raw)
				contexts[d.ID] = raw.Context
				queue = append(queue, &d)
				showNext()
			case "decision/resolved":
				delete(known, p.DecisionID)
				delete(contexts, p.DecisionID)
				if decision != nil && decision.ID == p.DecisionID {
					decision = nil
					addLine("Decision resolved.")
				}
				for i := 0; i < len(queue); i++ {
					if queue[i].ID == p.DecisionID {
						queue = append(queue[:i], queue[i+1:]...)
						break
					}
				}
				showNext()
			case "decision/reset":
				decision = nil
				queue = nil
				clear(known)
				clear(contexts)
				addLine("Pending decisions cleared.")

			case "error":
				addLine("Agent reported an error; check the session history.")
			default:
				continue
			}
			draw()
		case line := <-inputs:
			text := line.text
			needAck = line.err == nil
			if text == "" && line.err != nil {
				if line.err == io.EOF {
					return nil
				}
				return fmt.Errorf("terminal input: %w", line.err)
			}
			if text == "/quit" {
				return nil
			}
			if text == "/older" {
				if history.Next == nil || *history.Next == "" {
					addLine("No earlier messages.")
					draw()
				} else {
					cursor := *history.Next
					olderCursor = cursor
					history.Next = nil
					call("history", "threads/read", map[string]any{"thread_id": threadID, "workspace": workspace, "include_turns": true, "limit": 30, "cursor": cursor})
				}
				continue
			}
			if text == "/stop" {
				if busy != "" {
					call("stop", "turn/interrupt", map[string]any{"thread_id": threadID, "turn_id": busy})
				} else {
					addLine("No active turn to stop.")
					draw()
				}
				continue
			}
			if decision != nil && text != "" {
				d := decision
				if len(d.Context.Questions) > 0 {
					q := d.Context.Questions[question]
					answers[q.ID] = map[string]any{"answers": []string{text}}
					question++
					if question < len(d.Context.Questions) {
						addLine(cliSafeText(d.Context.Questions[question].Question))
						draw()
						continue
					}
					completedAnswers := answers
					decision = nil
					delete(known, d.ID)
					delete(contexts, d.ID)
					showNext()
					draw()
					call("decision", "decision/respond", map[string]any{"decision_id": d.ID, "result": map[string]any{"answers": completedAnswers}})
				} else {
					valid := false
					for _, choice := range d.Context.Choices {
						if text == choice && (choice == "accept" || choice == "decline" || choice == "cancel") {
							valid = true
						}
					}
					if !valid {
						addLine("Choose one of the offered decisions.")
						draw()
						continue
					}
					decision = nil
					delete(known, d.ID)
					delete(contexts, d.ID)
					showNext()
					draw()
					call("decision", "decision/respond", map[string]any{"decision_id": d.ID, "result": map[string]any{"decision": text}})
				}
				continue
			}
			if text != "" {
				if busy != "" || pending || activeUnknown {
					addLine("Wait for the active turn or use /stop.")
					draw()
				} else {
					pending = true
					completedSinceStart = false
					call("turn", "turn/start", map[string]any{"thread_id": threadID, "workspace": workspace, "text": text})
				}
			}
			if line.err != nil {
				if line.err == io.EOF {
					return nil
				}
				return fmt.Errorf("terminal input: %w", line.err)
			}
		}
	}
}

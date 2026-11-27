package antigravity

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/warmdev/modex/internal/acp"
)

const modelOption = "model"
const thinkingOption = "thought_level"

func restoreOptions(ctx context.Context, client *acp.Client, saved record) error {
	for _, choice := range []struct{ id, value string }{
		{modelOption, saved.Model}, {thinkingOption, saved.Thinking},
	} {
		if choice.value == "" {
			continue
		}
		for _, option := range client.Options(saved.Native) {
			if option.ID != choice.id || option.CurrentValue == choice.value {
				continue
			}
			if slices.ContainsFunc(option.Values, func(v acp.OptionValue) bool { return v.Value == choice.value }) {
				if err := client.SetOption(ctx, saved.Native, choice.id, choice.value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *Router) handleOptions(ctx context.Context, c *chat, method string, params map[string]json.RawMessage) (any, bool, error) {
	set := method == "threads/options/set"
	for key := range params {
		if key != "thread_id" && key != "workspace" && (!set || (key != "id" && key != "value")) {
			return nil, true, errors.New("unknown Antigravity option parameter")
		}
	}
	var id, value string
	if set && (json.Unmarshal(params["id"], &id) != nil ||
		(id != modelOption && id != thinkingOption) ||
		json.Unmarshal(params["value"], &value) != nil || !valid(value)) {
		return nil, true, errors.New("invalid Antigravity option")
	}
	client, err := r.load(ctx, c)
	if err != nil {
		return nil, true, err
	}
	if set {
		before := client.Options(c.Native)
		r.mu.Lock()
		if c.usageModel == "" {
			for _, option := range before {
				if option.ID == modelOption {
					c.usageModel = option.CurrentValue
				}
			}
		}
		if c.active != "" || c.setting || c.optionsDirty || !c.loaded {
			r.mu.Unlock()
			return nil, true, errors.New("Antigravity session is unavailable for changing options")
		}
		c.setting = true
		r.mu.Unlock()
		defer func() {
			r.mu.Lock()
			c.setting = false
			r.mu.Unlock()
		}()
		if err := client.SetOption(ctx, c.Native, id, value); err != nil {
			if !client.SessionLoaded(c.Native) {
				r.mu.Lock()
				c.loaded = false
				r.clearUsageLocked(c)
				c.usageModel = ""
				r.mu.Unlock()
			}
			return nil, true, err
		}
	}
	native := client.Options(c.Native)
	payload := make([]map[string]any, 0, len(native))
	for _, option := range native {
		if option.ID != modelOption && option.ID != thinkingOption {
			continue
		}
		values := make([]map[string]string, 0, len(option.Values))
		for _, value := range option.Values {
			values = append(values, map[string]string{"value": value.Value, "name": value.Name, "group": value.Group})
		}
		payload = append(payload, map[string]any{"id": option.ID, "name": option.Name, "currentValue": option.CurrentValue, "options": values})
	}
	if set {
		r.mu.Lock()
		for _, option := range native {
			if option.ID == modelOption && c.usageModel != "" && c.usageModel != option.CurrentValue {
				r.clearUsageLocked(c)
			}
			if option.ID == modelOption {
				c.usageModel = option.CurrentValue
			}
			switch option.ID {
			case modelOption:
				c.Model = option.CurrentValue
			case thinkingOption:
				c.Thinking = option.CurrentValue
			}
		}
		c.optionsDirty = true
		err = r.saveLocked()
		r.mu.Unlock()
		if err != nil {
			return nil, true, err
		}
	}
	return map[string]any{"thread_id": c.ID, "options": payload}, true, nil
}

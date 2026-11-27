package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/akutor/k1rothic-cli/internal/config"
	"github.com/akutor/k1rothic-cli/internal/core/provider"
	"github.com/akutor/k1rothic-cli/internal/core/provider/registry"
	"github.com/akutor/k1rothic-cli/internal/secrets"
)

// openAdapter builds the adapter for id from its saved settings.
func (m *Model) openAdapter(id string) (provider.Provider, error) {
	pc := m.cfg.ProviderSettings(id)
	return m.opts.Registry.Open(id, pc.BaseURL, pc.API)
}

// apiLabel is the human name of the API format id uses.
func (m *Model) apiLabel(id string) string {
	api := registry.APIFor(id, m.cfg.ProviderSettings(id).API)
	if a, ok := registry.LookupAPI(api); ok {
		return a.Label
	}
	return api
}

// setProviderConfig updates the saved section for id.
func (m *Model) setProviderConfig(id string, f func(pc *config.ProviderConfig)) {
	m.cfg.UpdateProvider(id, f)
	m.save()
}

// openAPIPicker lets the user choose the API format for id (custom
// endpoints, or OpenAI's Chat Completions vs Responses). then runs after a
// choice is made.
func (m *Model) openAPIPicker(id string, then func(m *Model) tea.Cmd) tea.Cmd {
	cur := registry.APIFor(id, m.cfg.ProviderSettings(id).API)
	p := &picker{title: "API format · " + m.providerLabel(id)}
	for _, api := range registry.APIsFor(id) {
		a, _ := registry.LookupAPI(api)
		d := a.Hint
		if api == cur && m.cfg.ProviderSettings(id).API != "" {
			d = "current · " + d
		}
		if api == cur {
			p.sel = len(p.items)
		}
		p.items = append(p.items, pickItem{value: api, label: a.Label, detail: d})
	}
	p.onPick = func(m *Model, it pickItem) tea.Cmd {
		changed := it.value != m.cfg.ProviderSettings(id).API
		m.setProviderConfig(id, func(pc *config.ProviderConfig) { pc.API = it.value })
		if changed {
			delete(m.modelCache, id) // a different API may list different models
		}
		if then != nil {
			return then(m)
		}
		return nil
	}
	m.panel = p
	return nil
}

// modelsMsg carries an async model listing.
type modelsMsg struct {
	provider string
	models   []provider.ModelInfo
	err      error
}

func (m *Model) providerLabel(id string) string {
	if i, ok := registry.Lookup(id); ok {
		return i.Label
	}
	return id
}

// keyDetail describes where a provider's key comes from.
func (m *Model) keyDetail(info registry.Info) string {
	if m.opts.Registry == nil || info.ID == "fake" {
		return "no key needed"
	}
	src, err := m.opts.Registry.KeyStatus(info.ID)
	switch {
	case err != nil:
		return "keychain unavailable"
	case src == secrets.Keychain:
		return "key in keychain"
	case src == secrets.Env:
		return "key from " + secrets.EnvVar(info.ID)
	case !info.KeyRequired:
		return "key optional"
	}
	return "no key"
}

// chooseProvider switches to id, first asking for whatever it still needs
// (API format and base URL for a custom endpoint, an API key for hosted providers).
func (m *Model) chooseProvider(id string) tea.Cmd {
	switch id {
	case actAddEndpoint, "custom", "add", "new":
		return m.startAddEndpoint()
	case actManage:
		return m.openEndpointsManager()
	}
	if name, ok := config.EndpointName(id); ok {
		if _, exists := m.cfg.Endpoints[name]; !exists {
			m.notice("No custom endpoint named "+name+". Try /endpoints.", true)
			return nil
		}
	} else if _, exists := m.cfg.Endpoints[id]; exists && !isBuiltin(id) {
		id = config.EndpointPrefix + id // "/provider local" for endpoint "local"
	}
	info, ok := registry.Lookup(id)
	if !ok {
		m.notice(fmt.Sprintf("Unknown provider %q. Try /provider.", id), true)
		return nil
	}
	if info.NeedsBaseURL && m.cfg.ProviderSettings(id).BaseURL == "" {
		// Unfinished custom endpoint: API format → base URL → key.
		return m.continueEndpointSetup(id)
	}
	if info.KeyRequired && m.opts.Registry != nil {
		if src, err := m.opts.Registry.KeyStatus(id); err == nil && src == secrets.None {
			m.openKeyPrompt(id, true)
			return nil
		}
	}
	return m.switchProvider(id)
}

func (m *Model) openKeyPrompt(id string, thenSwitch bool) {
	info, _ := registry.Lookup(id)
	help := []string{"Paste your " + info.Label + " API key. It is stored only in the OS keychain, never in a file."}
	if info.KeyURL != "" {
		help = append(help, "Create one at "+info.KeyURL)
	}
	if !info.KeyRequired {
		help = append(help, "Leave empty if the server needs no key.")
	}
	p := newPrompt(m, info.Label+" API key", "Key:", true, help...)
	p.allowEmpty = !info.KeyRequired
	p.onDone = func(m *Model, v string) tea.Cmd {
		if v != "" {
			if err := m.opts.Registry.Store.Set(id, v); err != nil {
				m.notice(err.Error(), true)
				return nil
			}
			m.notice(info.Label+" key saved to the OS keychain.", false)
		}
		if thenSwitch || id == m.cfg.Provider {
			return m.switchProvider(id) // reopen so the adapter picks up the key
		}
		return nil
	}
	m.panel = p
}

// openBaseURLPrompt asks for id's base URL, then for its key, then
// switches to it (the setup flow).
func (m *Model) openBaseURLPrompt(id string) {
	m.openBaseURLPromptThen(id, func(m *Model) tea.Cmd {
		m.openKeyPrompt(id, true)
		return nil
	})
}

// openBaseURLPromptThen asks for id's base URL and runs then after saving.
func (m *Model) openBaseURLPromptThen(id string, then func(m *Model) tea.Cmd) {
	api, _ := registry.LookupAPI(registry.APIFor(id, m.cfg.ProviderSettings(id).API))
	suffix := map[string]string{
		registry.APIChat:      "/chat/completions is added",
		registry.APIResponses: "/responses is added",
		registry.APIMessages:  "/v1/messages is added",
		registry.APIGemini:    "/v1beta/models/… is added",
	}[api.ID]
	p := newPrompt(m, m.providerLabel(id)+" · "+api.Label, "Base URL:", false,
		"The server's base URL, for example "+api.Example+" ("+suffix+").",
		api.Hint)
	p.input.SetValue(m.cfg.ProviderSettings(id).BaseURL)
	p.input.CursorEnd()
	p.onDone = func(m *Model, v string) tea.Cmd {
		if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
			m.notice("Base URL must start with http:// or https://", true)
			return nil
		}
		m.setProviderConfig(id, func(pc *config.ProviderConfig) { pc.BaseURL = strings.TrimRight(v, "/") })
		return then(m)
	}
	m.panel = p
}

// switchProvider makes id current, restoring its last model.
func (m *Model) switchProvider(id string) tea.Cmd {
	if m.opts.Registry == nil {
		m.notice("Provider switching is unavailable in this build.", true)
		return nil
	}
	info, _ := registry.Lookup(id)
	p, err := m.openAdapter(id)
	if err != nil {
		m.notice(err.Error(), true)
		return nil
	}
	if m.streaming {
		m.cancelStream()
	}
	m.prov = p
	m.cfg.Provider = id
	model := m.cfg.ProviderSettings(id).Model
	if model == "" {
		model = info.DefaultModel
	}
	m.cfg.Model = model
	m.updateContextLimit()
	m.save()
	if model == "" {
		m.notice("Provider: "+info.Label+". Pick a model.", false)
		return m.openModelPicker()
	}
	if len(registry.APIsFor(id)) > 1 {
		m.notice(fmt.Sprintf("Provider: %s (%s) · model %s", info.Label, m.apiLabel(id), model), false)
		return nil
	}
	m.notice(fmt.Sprintf("Provider: %s · model %s", info.Label, model), false)
	return nil
}

func (m *Model) openModelPicker() tea.Cmd {
	id := m.cfg.Provider
	p := &picker{title: "Model · " + m.providerLabel(id), custom: true, tag: "models:" + id,
		emptyMsg: "Type a model id and press Enter."}
	seen := map[string]bool{}
	add := func(v, detail string) {
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		p.items = append(p.items, pickItem{value: v, label: v, detail: detail})
	}
	add(m.cfg.Model, "current")
	if info, ok := registry.Lookup(id); ok {
		add(info.DefaultModel, "default")
	}
	if cached, ok := m.modelCache[id]; ok {
		p.items = append(p.items, m.modelItems(cached, seen)...)
	}
	p.onPick = func(m *Model, it pickItem) tea.Cmd { m.setModel(it.value); return nil }
	m.panel = p

	lister, ok := m.prov.(provider.ModelLister)
	if !ok || m.modelCache[id] != nil {
		return nil
	}
	p.loading = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		ms, err := lister.Models(ctx)
		return modelsMsg{provider: id, models: ms, err: err}
	}
}

func (m *Model) modelItems(ms []provider.ModelInfo, seen map[string]bool) []pickItem {
	var out []pickItem
	for _, mi := range ms {
		if seen[mi.ID] {
			continue
		}
		seen[mi.ID] = true
		d := mi.Name
		if cw := mi.ContextWindow; cw > 0 {
			if d != "" {
				d += " · "
			}
			d += FormatTokens(cw) + " context"
		}
		out = append(out, pickItem{value: mi.ID, label: mi.ID, detail: d})
	}
	return out
}

func (m *Model) handleModels(msg modelsMsg) {
	if msg.err == nil {
		m.modelCache[msg.provider] = msg.models
		m.updateContextLimit()
	}
	p, ok := m.panel.(*picker)
	if !ok || p.tag != "models:"+msg.provider {
		return
	}
	p.loading = false
	if msg.err != nil {
		p.err = "could not list models: " + msg.err.Error()
		return
	}
	seen := map[string]bool{}
	for _, it := range p.items {
		seen[it.value] = true
	}
	p.items = append(p.items, m.modelItems(msg.models, seen)...)
	// Fill in details (context size) for the pinned current/default rows.
	for i, it := range p.items {
		for _, mi := range msg.models {
			if mi.ID == it.value && mi.ContextWindow > 0 && !strings.Contains(it.detail, "context") {
				p.items[i].detail = it.detail + " · " + FormatTokens(mi.ContextWindow) + " context"
			}
		}
	}
}

func (m *Model) setModel(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	m.cfg.Model = id
	m.cfg.SetProviderModel(m.cfg.Provider, id)
	m.updateContextLimit()
	m.save()
	m.notice("Model: "+id, false)
}

// updateContextLimit derives the context window for the current model from
// the provider's listing, falling back to the static table.
func (m *Model) updateContextLimit() {
	if m.opts.ContextLimit > 0 {
		m.ctxLimit = m.opts.ContextLimit
		return
	}
	for _, mi := range m.modelCache[m.cfg.Provider] {
		if mi.ID == m.cfg.Model && mi.ContextWindow > 0 {
			m.ctxLimit = mi.ContextWindow
			return
		}
	}
	m.ctxLimit = registry.ContextWindow(m.cfg.Model)
}

// save persists the config; failures are shown but not fatal.
func (m *Model) save() {
	if m.opts.ConfigPath == "" {
		return
	}
	if err := config.Save(m.opts.ConfigPath, m.cfg); err != nil {
		m.notice("Could not save settings: "+err.Error(), true)
	}
}

// providerCommand handles "/provider [id|key|remove-key]".
func (m *Model) providerCommand(args string) tea.Cmd {
	switch args {
	case "":
		return m.openProviderPicker()
	case "key":
		if m.cfg.Provider == "fake" {
			m.notice("The demo provider needs no key.", false)
			return nil
		}
		m.openKeyPrompt(m.cfg.Provider, false)
		return nil
	case "api":
		if len(registry.APIsFor(m.cfg.Provider)) < 2 {
			m.notice(m.providerLabel(m.cfg.Provider)+" only speaks "+m.apiLabel(m.cfg.Provider)+".", false)
			return nil
		}
		return m.openAPIPicker(m.cfg.Provider, func(m *Model) tea.Cmd { return m.switchProvider(m.cfg.Provider) })
	case "url":
		if info, _ := registry.Lookup(m.cfg.Provider); !info.NeedsBaseURL {
			m.notice("Only custom endpoints have a configurable URL — see /endpoints.", false)
			return nil
		}
		m.openBaseURLPrompt(m.cfg.Provider)
		return nil
	case "remove-key":
		if m.opts.Registry == nil {
			return nil
		}
		if err := m.opts.Registry.Store.Delete(m.cfg.Provider); err != nil {
			m.notice(err.Error(), true)
			return nil
		}
		m.notice(m.providerLabel(m.cfg.Provider)+" key removed from the keychain.", false)
		return m.switchProvider(m.cfg.Provider)
	}
	return m.chooseProvider(args)
}

func isBuiltin(id string) bool { _, ok := registry.Lookup(id); return ok && !registry.IsEndpoint(id) }

// startupNotice tells the user what is missing before the first request.
func (m *Model) startupNotice() {
	info, ok := registry.Lookup(m.cfg.Provider)
	if !ok || m.opts.Registry == nil || !info.KeyRequired {
		return
	}
	src, err := m.opts.Registry.KeyStatus(info.ID)
	switch {
	case err != nil:
		m.notice(fmt.Sprintf("OS keychain unavailable (%v). Set %s for this session, or choose another provider with /provider.", err, secrets.EnvVar(info.ID)), true)
	case src == secrets.None:
		m.notice(fmt.Sprintf("No %s API key yet. Run /provider to add one (stored in the OS keychain), or /settings for everything else.", info.Label), false)
	}
}

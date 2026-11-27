package ui

import (
	"fmt"
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/akutor/k1rothic-cli/internal/config"
	"github.com/akutor/k1rothic-cli/internal/core/provider/registry"
)

// Picker values for the actions that are not providers.
const (
	actAddEndpoint = "+add"
	actManage      = "+manage"
)

// endpointIDs returns the provider ids of all custom endpoints, by name.
func (m *Model) endpointIDs() []string {
	names := make([]string, 0, len(m.cfg.Endpoints))
	for n := range m.cfg.Endpoints {
		names = append(names, n)
	}
	sort.Strings(names)
	ids := make([]string, len(names))
	for i, n := range names {
		ids[i] = config.EndpointPrefix + n
	}
	return ids
}

// endpointDetail summarises an endpoint for lists.
func (m *Model) endpointDetail(id string) string {
	pc := m.cfg.ProviderSettings(id)
	info, _ := registry.Lookup(id)
	if pc.BaseURL == "" {
		return "setup not finished · " + m.keyDetail(info)
	}
	return m.apiLabel(id) + " · " + pc.BaseURL + " · " + m.keyDetail(info)
}

func (m *Model) openProviderPicker() tea.Cmd {
	p := &picker{title: "Provider", altHint: "Tab key / edit"}
	add := func(id, label, detail string) {
		if id == m.cfg.Provider {
			detail += " · current"
			p.sel = len(p.items)
		}
		p.items = append(p.items, pickItem{value: id, label: label, detail: detail})
	}
	for _, info := range registry.All {
		add(info.ID, info.Label, m.keyDetail(info))
	}
	for _, id := range m.endpointIDs() {
		name, _ := config.EndpointName(id)
		add(id, name, m.endpointDetail(id))
	}
	p.items = append(p.items, pickItem{value: actAddEndpoint, label: "+ Add custom endpoint",
		detail: "any server speaking the OpenAI, Anthropic or Gemini API"})
	if len(m.cfg.Endpoints) > 0 {
		p.items = append(p.items, pickItem{value: actManage, label: "  Manage endpoints", detail: "edit or delete custom endpoints"})
	}
	p.onPick = func(m *Model, it pickItem) tea.Cmd { return m.chooseProvider(it.value) }
	p.onAlt = func(m *Model, it pickItem) tea.Cmd {
		switch {
		case registry.IsEndpoint(it.value):
			return m.openEndpointMenu(it.value)
		case it.value == "fake" || it.value == actAddEndpoint || it.value == actManage:
			return nil
		}
		m.openKeyPrompt(it.value, false)
		return nil
	}
	m.panel = p
	return nil
}

// startAddEndpoint asks for a name, then runs the setup: API format → base
// URL → key → switch.
func (m *Model) startAddEndpoint() tea.Cmd {
	p := newPrompt(m, "New custom endpoint", "Name:", false,
		"A short name for this server, e.g. local, deepseek, work-proxy (letters, digits, - and _).")
	p.onDone = func(m *Model, v string) tea.Cmd {
		if !config.ValidEndpointName(v) {
			m.notice("Endpoint names may use letters, digits, '-' and '_' (max 32).", true)
			return m.startAddEndpoint()
		}
		if _, exists := m.cfg.Endpoints[v]; exists {
			m.notice("An endpoint named "+v+" already exists — pick another name or edit it with /endpoints.", true)
			return m.startAddEndpoint()
		}
		id := config.EndpointPrefix + v
		m.setProviderConfig(id, func(*config.ProviderConfig) {})
		return m.continueEndpointSetup(id)
	}
	m.panel = p
	return nil
}

// continueEndpointSetup resumes an unfinished endpoint: API format first,
// then the base URL (which then asks for the key and switches to it).
func (m *Model) continueEndpointSetup(id string) tea.Cmd {
	return m.openAPIPicker(id, func(m *Model) tea.Cmd {
		m.openBaseURLPrompt(id)
		return nil
	})
}

// openEndpointsManager lists the custom endpoints for editing.
func (m *Model) openEndpointsManager() tea.Cmd {
	p := &picker{title: "Custom endpoints", emptyMsg: "No custom endpoints yet."}
	for _, id := range m.endpointIDs() {
		name, _ := config.EndpointName(id)
		d := m.endpointDetail(id)
		if id == m.cfg.Provider {
			d += " · current"
		}
		p.items = append(p.items, pickItem{value: id, label: name, detail: d})
	}
	p.items = append(p.items, pickItem{value: actAddEndpoint, label: "+ Add custom endpoint", detail: "OpenAI, Anthropic or Gemini API"})
	p.onPick = func(m *Model, it pickItem) tea.Cmd {
		if it.value == actAddEndpoint {
			return m.startAddEndpoint()
		}
		return m.openEndpointMenu(it.value)
	}
	m.panel = p
	return nil
}

// openEndpointMenu shows what can be done with one endpoint.
func (m *Model) openEndpointMenu(id string) tea.Cmd {
	name, _ := config.EndpointName(id)
	pc := m.cfg.ProviderSettings(id)
	url := pc.BaseURL
	if url == "" {
		url = "not set"
	}
	info, _ := registry.Lookup(id)
	p := &picker{title: "Endpoint · " + name}
	p.items = []pickItem{
		{value: "use", label: "Use this endpoint", detail: m.apiLabel(id)},
		{value: "api", label: "Change API format", detail: m.apiLabel(id)},
		{value: "url", label: "Change base URL", detail: url},
		{value: "key", label: "Set or replace API key", detail: m.keyDetail(info)},
		{value: "remove-key", label: "Remove API key", detail: "keeps the endpoint"},
		{value: "delete", label: "Delete endpoint", detail: "removes it and its saved key"},
	}
	p.onPick = func(m *Model, it pickItem) tea.Cmd {
		switch it.value {
		case "use":
			return m.chooseProvider(id)
		case "api":
			return m.openAPIPicker(id, func(m *Model) tea.Cmd {
				if id == m.cfg.Provider {
					return m.switchProvider(id)
				}
				m.notice(name+": API format set to "+m.apiLabel(id)+".", false)
				return nil
			})
		case "url":
			m.openBaseURLPromptThen(id, func(m *Model) tea.Cmd {
				if id == m.cfg.Provider {
					return m.switchProvider(id)
				}
				m.notice(name+": base URL updated.", false)
				return nil
			})
		case "key":
			m.openKeyPrompt(id, false)
		case "remove-key":
			if err := m.opts.Registry.Store.Delete(id); err != nil {
				m.notice(err.Error(), true)
				return nil
			}
			m.notice(name+": key removed from the keychain.", false)
			if id == m.cfg.Provider {
				return m.switchProvider(id)
			}
		case "delete":
			return m.confirmDeleteEndpoint(id)
		}
		return nil
	}
	m.panel = p
	return nil
}

func (m *Model) confirmDeleteEndpoint(id string) tea.Cmd {
	name, _ := config.EndpointName(id)
	p := &picker{title: "Delete endpoint " + name + "?"}
	p.items = []pickItem{
		{value: "no", label: "No, keep it"},
		{value: "yes", label: "Yes, delete " + name, detail: "removes its settings and the key saved in the keychain"},
	}
	p.onPick = func(m *Model, it pickItem) tea.Cmd {
		if it.value != "yes" {
			return m.openEndpointMenu(id)
		}
		return m.deleteEndpoint(id)
	}
	m.panel = p
	return nil
}

func (m *Model) deleteEndpoint(id string) tea.Cmd {
	name, _ := config.EndpointName(id)
	if m.opts.Registry != nil {
		if err := m.opts.Registry.Store.Delete(id); err != nil {
			m.notice(err.Error(), true) // settings are still removed below
		}
	}
	delete(m.cfg.Endpoints, name)
	if len(m.cfg.Endpoints) == 0 {
		m.cfg.Endpoints = nil
	}
	delete(m.modelCache, id)
	m.notice(fmt.Sprintf("Endpoint %s deleted.", name), false)
	if id == m.cfg.Provider {
		// The current provider is gone: fall back to Anthropic.
		return m.switchProvider("anthropic")
	}
	m.save()
	return nil
}

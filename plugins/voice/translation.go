package voice

import "github.com/c/just-talk-go/hotkey"

// One physical chord can notify both registered combinations, on separate
// dispatcher goroutines. Route the full native snapshot once per sequence.
type translationChord struct {
	sequence         uint64
	held, mayPromote bool
}

type chordAction struct {
	emit, translate, promote bool
	edge                     hotkey.EventType
}

func (c *translationChord) step(evt hotkey.Event, base, extra hotkey.Modifier, mode string, active bool) chordAction {
	if evt.Sequence <= c.sequence {
		return chordAction{}
	}
	c.sequence = evt.Sequence
	held := evt.Modifiers&base == base
	translate := (evt.Modifiers|evt.ModifierHistory)&extra == extra
	previous := c.held
	c.held = held
	if held && !previous {
		c.mayPromote = mode == "hold" || !active
		return chordAction{emit: true, edge: hotkey.KeyDown, translate: translate}
	}
	if held && previous {
		return chordAction{promote: translate && c.mayPromote}
	}
	if previous && !held {
		promote := translate && c.mayPromote
		c.mayPromote = false
		return chordAction{emit: mode == "hold", edge: hotkey.KeyUp, promote: promote}
	}
	return chordAction{}
}

func (p *VoicePlugin) handleTranslationChord(evt hotkey.Event) bool {
	p.mu.Lock()
	if p.cfg == nil || evt.Sequence == 0 {
		p.mu.Unlock()
		return false
	}
	if evt.Combo != p.combo && (!p.cfg.Translation.Enabled || evt.Combo != p.translationCombo) {
		p.mu.Unlock()
		return true
	}
	if !p.cfg.Translation.Enabled {
		p.mu.Unlock()
		return false
	}
	action := p.translationChord.step(evt, p.combo.Mods, p.translationCombo.Mods, p.mode, p.recording || p.starting)
	p.mu.Unlock()
	if action.promote {
		p.promoteTranslation()
	}
	if action.emit {
		p.handleVoiceEdge(action.edge, action.translate)
	}
	return true
}

func (p *VoicePlugin) promoteTranslation() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if (p.starting || p.recording) && p.sessionTranslation.Enabled && !p.sessionTranslate {
		p.sessionTranslate = true
		p.publishStatusLocked()
	}
}

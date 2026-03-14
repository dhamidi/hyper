package htmlc

import (
	"fmt"
	"strings"
)

type nodeType int

const (
	elementNode nodeType = iota
	textNode
)

type node struct {
	typ      nodeType
	tag      string
	attrs    []attr
	children []*node
	text     string // for textNode
}

type attr struct {
	name  string
	value string
}

// extractTemplate extracts the inner content of the top-level <template> block
// from a .vue file.
func extractTemplate(content string) (string, error) {
	idx := strings.Index(content, "<template")
	if idx < 0 {
		return "", fmt.Errorf("no <template> block found")
	}
	gt := strings.Index(content[idx:], ">")
	if gt < 0 {
		return "", fmt.Errorf("unclosed <template> tag")
	}
	start := idx + gt + 1

	end := strings.LastIndex(content, "</template>")
	if end < 0 || end < start {
		return "", fmt.Errorf("no closing </template> tag")
	}

	return content[start:end], nil
}

// parseTemplate parses an HTML template string into a tree of nodes.
func parseTemplate(tmpl string) ([]*node, error) {
	p := &templateParser{input: tmpl}
	return p.parseNodes("")
}

type templateParser struct {
	input string
	pos   int
}

func (p *templateParser) parseNodes(until string) ([]*node, error) {
	var nodes []*node
	for p.pos < len(p.input) {
		if until != "" && p.lookingAt("</"+until+">") {
			break
		}

		if p.lookingAt("<!--") {
			p.skipComment()
			continue
		}

		if p.lookingAt("</") {
			// Unexpected closing tag; stop parsing this level
			break
		}

		if p.input[p.pos] == '<' {
			n, err := p.parseElement()
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, n)
		} else {
			text := p.parseText()
			if text != "" {
				nodes = append(nodes, &node{typ: textNode, text: text})
			}
		}
	}
	return nodes, nil
}

func (p *templateParser) parseElement() (*node, error) {
	p.pos++ // skip '<'

	tag := p.readTagName()
	if tag == "" {
		return nil, fmt.Errorf("htmlc: empty tag name at position %d", p.pos)
	}

	attrs, selfClosing, err := p.parseAttrs()
	if err != nil {
		return nil, err
	}

	n := &node{typ: elementNode, tag: tag, attrs: attrs}

	if selfClosing || isVoidElement(tag) {
		return n, nil
	}

	children, err := p.parseNodes(tag)
	if err != nil {
		return nil, err
	}
	n.children = children

	// Consume closing tag
	closeTag := "</" + tag + ">"
	if p.lookingAt(closeTag) {
		p.pos += len(closeTag)
	}

	return n, nil
}

func (p *templateParser) parseAttrs() ([]attr, bool, error) {
	var attrs []attr
	selfClosing := false

	for p.pos < len(p.input) {
		p.skipWhitespace()
		if p.pos >= len(p.input) {
			break
		}

		if p.input[p.pos] == '/' && p.pos+1 < len(p.input) && p.input[p.pos+1] == '>' {
			p.pos += 2
			selfClosing = true
			return attrs, selfClosing, nil
		}

		if p.input[p.pos] == '>' {
			p.pos++
			return attrs, selfClosing, nil
		}

		name := p.readAttrName()
		if name == "" {
			break
		}

		p.skipWhitespace()
		if p.pos < len(p.input) && p.input[p.pos] == '=' {
			p.pos++ // skip '='
			p.skipWhitespace()
			value, err := p.readAttrValue()
			if err != nil {
				return nil, false, err
			}
			attrs = append(attrs, attr{name: name, value: value})
		} else {
			attrs = append(attrs, attr{name: name, value: ""})
		}
	}

	return attrs, selfClosing, nil
}

func (p *templateParser) readTagName() string {
	start := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if isTagChar(c) {
			p.pos++
		} else {
			break
		}
	}
	return p.input[start:p.pos]
}

func (p *templateParser) readAttrName() string {
	start := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == '=' || c == '>' || c == '/' || c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			break
		}
		p.pos++
	}
	return p.input[start:p.pos]
}

func (p *templateParser) readAttrValue() (string, error) {
	if p.pos >= len(p.input) {
		return "", nil
	}

	quote := p.input[p.pos]
	if quote == '"' || quote == '\'' {
		p.pos++ // skip opening quote
		start := p.pos
		for p.pos < len(p.input) && p.input[p.pos] != quote {
			p.pos++
		}
		value := p.input[start:p.pos]
		if p.pos < len(p.input) {
			p.pos++ // skip closing quote
		}
		return value, nil
	}

	// Unquoted value
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != ' ' && p.input[p.pos] != '>' {
		p.pos++
	}
	return p.input[start:p.pos], nil
}

func (p *templateParser) parseText() string {
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != '<' {
		p.pos++
	}
	return p.input[start:p.pos]
}

func (p *templateParser) skipComment() {
	p.pos += 4 // skip "<!--"
	end := strings.Index(p.input[p.pos:], "-->")
	if end < 0 {
		p.pos = len(p.input)
	} else {
		p.pos += end + 3
	}
}

func (p *templateParser) skipWhitespace() {
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			p.pos++
		} else {
			break
		}
	}
}

func (p *templateParser) lookingAt(s string) bool {
	return strings.HasPrefix(p.input[p.pos:], s)
}

func isTagChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '_'
}

func isVoidElement(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}

// isHTMLElement returns true for standard HTML element names.
// These are never treated as component references.
func isHTMLElement(tag string) bool {
	switch tag {
	case "a", "abbr", "address", "area", "article", "aside", "audio",
		"b", "base", "bdi", "bdo", "blockquote", "body", "br", "button",
		"canvas", "caption", "cite", "code", "col", "colgroup",
		"data", "datalist", "dd", "del", "details", "dfn", "dialog", "div", "dl", "dt",
		"em", "embed",
		"fieldset", "figcaption", "figure", "footer", "form",
		"h1", "h2", "h3", "h4", "h5", "h6", "head", "header", "hgroup", "hr", "html",
		"i", "iframe", "img", "input", "ins",
		"kbd",
		"label", "legend", "li", "link",
		"main", "map", "mark", "math", "menu", "meta", "meter",
		"nav", "noscript",
		"object", "ol", "optgroup", "option", "output",
		"p", "param", "picture", "pre", "progress",
		"q",
		"rp", "rt", "ruby",
		"s", "samp", "script", "search", "section", "select", "slot", "small", "source", "span", "strong", "style", "sub", "summary", "sup", "svg",
		"table", "tbody", "td", "template", "textarea", "tfoot", "th", "thead", "time", "title", "tr", "track",
		"u", "ul",
		"var", "video",
		"wbr":
		return true
	}
	return false
}

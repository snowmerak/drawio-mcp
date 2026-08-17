package drawio

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// Node is a small XML DOM used to preserve draw.io attributes and children.
type Node struct {
	Name     xml.Name
	Attrs    []xml.Attr
	Children []*Node
	Text     string
}

func (n *Node) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	n.Name = start.Name
	n.Attrs = append(n.Attrs[:0], start.Attr...)
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.StartElement:
			child := &Node{}
			if err := decoder.DecodeElement(child, &value); err != nil {
				return err
			}
			n.Children = append(n.Children, child)
		case xml.CharData:
			n.Text += string(value)
		case xml.EndElement:
			return nil
		}
	}
}

func (n Node) MarshalXML(encoder *xml.Encoder, _ xml.StartElement) error {
	start := xml.StartElement{Name: n.Name, Attr: n.Attrs}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	if n.Text != "" {
		if err := encoder.EncodeToken(xml.CharData(n.Text)); err != nil {
			return err
		}
	}
	for _, child := range n.Children {
		if err := encoder.Encode(child); err != nil {
			return err
		}
	}
	return encoder.EncodeToken(start.End())
}

func parseNode(data []byte) (*Node, error) {
	var node Node
	if err := xml.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

func (n *Node) attr(name string) string {
	for _, attr := range n.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func (n *Node) setAttr(name, value string) {
	for i := range n.Attrs {
		if n.Attrs[i].Name.Local == name {
			n.Attrs[i].Value = value
			return
		}
	}
	n.Attrs = append(n.Attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

func (n *Node) child(name string) *Node {
	for _, child := range n.Children {
		if child.Name.Local == name {
			return child
		}
	}
	return nil
}

func element(name string, attrs ...string) *Node {
	if len(attrs)%2 != 0 {
		panic(fmt.Sprintf("attributes for %s must be key/value pairs", name))
	}
	node := &Node{Name: xml.Name{Local: name}}
	for i := 0; i < len(attrs); i += 2 {
		node.setAttr(attrs[i], attrs[i+1])
	}
	return node
}

func compactText(value string) string {
	return strings.TrimSpace(value)
}

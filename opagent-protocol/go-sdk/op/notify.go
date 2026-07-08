package op

import "encoding/json"

type InfoNotificationParams struct {
	OpCode  OpCode `json:"opcode"`
	Meta    `json:"_meta,omitempty"`
	Content Content `json:"content"`
}

func (*InfoNotificationParams) isParams() {}

// UnmarshalJSON handles the unmarshalling of content into the Content interface.
func (p *InfoNotificationParams) UnmarshalJSON(data []byte) error {
	type params InfoNotificationParams // avoid recursion
	var wire struct {
		params
		Content *wireContent `json:"content"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	var err error
	if wire.params.Content, err = contentFromWire(wire.Content, nil); err != nil {
		return err
	}
	*p = InfoNotificationParams(wire.params)
	return nil
}

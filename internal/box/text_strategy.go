package box

// TextStrategy handles plain text files with no transformation or validation.
type TextStrategy struct{}

func (t *TextStrategy) Transform(value string) string { return value }
func (t *TextStrategy) Validate(_ string) error       { return nil }
func (t *TextStrategy) SchemaType() SchemaType        { return TXT }
func (t *TextStrategy) Format(content []byte) ([]byte, error) {
	return content, nil
}

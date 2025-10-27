package static

import _ "embed"

//go:embed fonts/AtkinsonHyperlegible-Regular.ttf
var AtkinsonRegular []byte

//go:embed fonts/AtkinsonHyperlegible-Bold.ttf
var AtkinsonBold []byte

//go:embed templates/wrapper.html
var WrapperTemplate string

//go:embed templates/source.html
var SourceTemplate string

//go:embed fonts/OFL.txt
var FontLicense string

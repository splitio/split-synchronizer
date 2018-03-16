package HTMLtemplates

import (
	"bytes"
	"testing"

	"github.com/alecthomas/template"
)

func TestMainMenu(t *testing.T) {

	buf := new(bytes.Buffer)

	tpl := template.Must(template.New("MainMenu").Parse(MainMenuTPL))

	tpl.Execute(buf, MainMenuTPLVars{ProxyMode: false})
	if len(buf.String()) != 692 {
		t.Error("Parssed main menu ProxyMode:FALSE wrong len")
	}

	buf.Reset()
	tpl.Execute(buf, MainMenuTPLVars{ProxyMode: true})
	if len(buf.String()) != 887 {
		t.Error("Parssed main menu ProxyMode:TRUE wrong len")
	}
}

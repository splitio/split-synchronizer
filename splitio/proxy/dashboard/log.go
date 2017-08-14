package dashboard

import "fmt"

var logRow = `<tr>
  <td>%s</td>
</tr>`

// ParseLastErrors return HTML table row per message
func ParseLastErrors(messages []string) string {
	var html = ""

	for _, message := range messages {
		html += fmt.Sprintf(logRow, message)
	}

	return html
}

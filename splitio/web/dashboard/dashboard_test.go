package dashboard

import (
	"fmt"
	"testing"
)

func TestDashboardHTML(t *testing.T) {
	dash := NewDashboard(true)
	fmt.Println(dash.HTML())
}

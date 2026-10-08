package cron

import (
	"strings"
	"testing"

	"olspanel/internal/db"
)

func TestRender(t *testing.T) {
	out := Render([]db.CronJob{
		{Schedule: "*/5 * * * *", Command: "php /home/a/x.php", Enabled: true},
		{Schedule: "0 3 * * *", Command: "date +%Y", Enabled: false},
	})
	if !strings.HasPrefix(out, "# Zarządzane przez olspanel") {
		t.Errorf("missing header")
	}
	if !strings.Contains(out, `MAILTO=""`) {
		t.Errorf("missing MAILTO")
	}
	if !strings.Contains(out, "*/5 * * * * php /home/a/x.php\n") {
		t.Errorf("enabled job missing:\n%s", out)
	}
	if !strings.Contains(out, "#DISABLED 0 3 * * * date +\\%Y\n") {
		t.Errorf("disabled job / percent escaping wrong:\n%s", out)
	}
}

func TestRequestValidate(t *testing.T) {
	good := Request{Schedule: " */10  *  * * * ", Command: "echo ok"}
	if err := good.validate(); err != nil {
		t.Fatal(err)
	}
	if good.Schedule != "*/10 * * * *" {
		t.Errorf("schedule not normalised: %q", good.Schedule)
	}
	for _, bad := range []Request{
		{Schedule: "* * * *", Command: "x"},
		{Schedule: "99 * * * *", Command: "x"},
		{Schedule: "* * * * *", Command: ""},
		{Schedule: "* * * * *", Command: "a\nb"},
		{Schedule: "@reboot", Command: "x"},
	} {
		if err := bad.validate(); err == nil {
			t.Errorf("%+v should be invalid", bad)
		}
	}
}

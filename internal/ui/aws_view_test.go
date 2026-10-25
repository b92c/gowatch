package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/b92c/gowatch/internal/aws"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// noCredentials is the problem the AWS manager reports when the SDK finds no
// credential provider.
var noCredentials = aws.Problem{Text: "no AWS credentials found", Hint: "configure the AWS CLI or set aws.enabled: false"}

func TestAWSViewTitle(t *testing.T) {
	tests := []struct {
		name string
		want string
		st   aws.AWSState
	}{
		{name: "the zero state", st: aws.AWSState{}, want: " AWS Cloud Resources (us-east-1) not configured "},
		{name: "not configured", st: aws.AWSState{Region: "eu-west-1", Conn: aws.ConnNotConfigured}, want: " AWS Cloud Resources (eu-west-1) not configured "},
		{name: "disabled", st: aws.AWSState{Conn: aws.ConnDisabled}, want: " AWS Cloud Resources (us-east-1) disabled "},
		{name: "connecting", st: aws.AWSState{Conn: aws.ConnConnecting}, want: " AWS Cloud Resources (us-east-1) connecting "},
		{
			name: "connected",
			st:   aws.AWSState{Region: "sa-east-1", Conn: aws.ConnConnected, Identity: aws.Identity{AccountID: "123456789012"}},
			want: " AWS Cloud Resources (sa-east-1) connected: 123456789012 ",
		},
		{
			name: "an error shows its text and never its hint",
			st:   aws.AWSState{Conn: aws.ConnError, LastError: noCredentials},
			want: " AWS Cloud Resources (us-east-1) error: no AWS credentials found ",
		},
		{
			name: "an error shows its text up to the first colon",
			st:   aws.AWSState{Conn: aws.ConnError, LastError: aws.Problem{Text: "AWS unreachable: timeout"}},
			want: " AWS Cloud Resources (us-east-1) error: AWS unreachable ",
		},
		{
			name: "a failing identity does not show the discovery problem it keeps",
			st: aws.AWSState{
				Conn:           aws.ConnError,
				LastError:      aws.Problem{Text: "credentials expired or invalid", Hint: "log in again and re-export the keys"},
				DiscoveryError: aws.Problem{Text: "SSO token expired", Hint: "run aws sso login"},
			},
			want: " AWS Cloud Resources (us-east-1) error: credentials expired or invalid ",
		},
		{name: "the region is escaped", st: aws.AWSState{Region: "x[y]", Conn: aws.ConnConnecting}, want: " AWS Cloud Resources (x[y[]) connecting "},
		{
			name: "the error text is escaped",
			st:   aws.AWSState{Conn: aws.ConnError, LastError: aws.Problem{Text: "failed [red]"}},
			want: " AWS Cloud Resources (us-east-1) error: failed [red[] ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := awsTitle(tt.st); got != tt.want {
				t.Errorf("awsTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAWSViewExplanation(t *testing.T) {
	tests := []struct {
		name string
		want string
		st   aws.AWSState
	}{
		{name: "the zero state", st: aws.AWSState{}, want: explainNotConfigured},
		{name: "not configured", st: aws.AWSState{Conn: aws.ConnNotConfigured}, want: explainNotConfigured},
		{name: "disabled", st: aws.AWSState{Conn: aws.ConnDisabled}, want: explainDisabled},
		{name: "connecting", st: aws.AWSState{Conn: aws.ConnConnecting}, want: explainConnecting},
		{name: "an error with its hint", st: aws.AWSState{Conn: aws.ConnError, LastError: noCredentials}, want: "no AWS credentials found - configure the AWS CLI or set aws.enabled: false"},
		{name: "connected without rows", st: aws.AWSState{Conn: aws.ConnConnected}, want: explainNoRows},
		{name: "an error text is escaped", st: aws.AWSState{Conn: aws.ConnError, LastError: aws.Problem{Text: "failed [red]"}}, want: "failed [red[]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDashboard(t, Options{})
			d.awsViewMode = true

			d.updateAWSTable(tt.st)

			if got := d.servicesTable.GetRowCount(); got != 2 {
				t.Fatalf("rows = %d, want the header and one explanatory row", got)
			}
			cell := d.servicesTable.GetCell(1, 0)
			if cell.Text != tt.want {
				t.Errorf("explanation = %q, want %q", cell.Text, tt.want)
			}
			if got := fgOf(cell); got != d.theme.Muted {
				t.Errorf("explanation color = %v, want the muted color %v", got, d.theme.Muted)
			}
			if !cell.NotSelectable {
				t.Error("the explanatory row must not be selectable")
			}
			for col := 1; col < 6; col++ {
				if text := d.servicesTable.GetCell(1, col).Text; text != "" {
					t.Errorf("column %d of the explanatory row = %q, want empty", col, text)
				}
			}
		})
	}
}

// The explanation is a single cell in the first column, so it must not be cut
// by a narrow terminal.
func TestAWSViewExplanationFitsEightyColumns(t *testing.T) {
	tests := []struct {
		name string
		want string
		st   aws.AWSState
	}{
		{name: "not configured", st: aws.AWSState{Conn: aws.ConnNotConfigured}, want: explainNotConfigured},
		{name: "disabled", st: aws.AWSState{Conn: aws.ConnDisabled}, want: explainDisabled},
		{name: "no credentials", st: aws.AWSState{Conn: aws.ConnError, LastError: noCredentials}, want: "no AWS credentials found - configure the AWS CLI or set aws.enabled: false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDashboard(t, Options{})
			d.awsViewMode = true
			containers := sampleContainers()
			containers.AWS = tt.st
			d.Update(containers)

			shot := drawDashboard(t, d, 80, 30)

			if _, _, ok := shot.find(tt.want); !ok {
				t.Errorf("%q is not fully on screen at 80 columns:\n%s", tt.want, shot.text())
			}
		})
	}
}

func TestAWSViewAccountRow(t *testing.T) {
	d := newTestDashboard(t, Options{})
	d.awsViewMode = true
	const arn = "arn:aws:sts::123456789012:assumed-role/ReadOnly/dev"
	st := aws.AWSState{
		Conn:     aws.ConnConnected,
		Region:   "sa-east-1",
		Identity: aws.Identity{AccountID: "123456789012", ARN: arn},
		Accounts: []aws.Account{{
			ID: "123456789012", Name: "123456789012", Status: aws.StatusMonitored, Identity: true,
			UpdatedAt: time.Date(2026, 10, 25, 9, 8, 7, 0, time.Local),
		}},
	}

	d.updateAWSTable(st)

	if got := d.servicesTable.GetRowCount(); got != 2 {
		t.Fatalf("rows = %d, want the header and one account", got)
	}
	want := []struct {
		text  string
		color tcell.Color
	}{
		{text: "123456789012", color: d.theme.Text},
		{text: "Account", color: d.theme.Info},
		{text: "sa-east-1", color: d.theme.Muted},
		{text: "monitored", color: d.theme.StatusOK},
		{text: "123456789012; " + arn, color: d.theme.Text},
		{text: "09:08:07", color: d.theme.Muted},
	}
	for col, w := range want {
		cell := d.servicesTable.GetCell(1, col)
		if cell.Text != w.text || fgOf(cell) != w.color {
			t.Errorf("column %d = %q in %v, want %q in %v", col, cell.Text, fgOf(cell), w.text, w.color)
		}
	}
	if got := d.servicesTable.GetCell(1, 0).MaxWidth; got != awsNameMaxWidth {
		t.Errorf("name MaxWidth = %d, want %d", got, awsNameMaxWidth)
	}
	for col, header := range []string{"Resource Name", "Type", "Region", "Status", "Metrics Summary", "Last Updated"} {
		cell := d.servicesTable.GetCell(0, col)
		if cell.Text != header || fgOf(cell) != d.theme.Accent || cell.Align != tview.AlignCenter || !cell.NotSelectable {
			t.Errorf("header %d = %q in %v (align %d, not selectable %t), want an accent, centered, not selectable %q",
				col, cell.Text, fgOf(cell), cell.Align, cell.NotSelectable, header)
		}
	}
	if got, want := d.servicesTable.GetTitle(), " AWS Cloud Resources (sa-east-1) connected: 123456789012 "; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
}

func TestAWSAccountSummary(t *testing.T) {
	id := aws.Identity{AccountID: "123456789012", ARN: "arn:aws:sts::123456789012:assumed-role/ReadOnly/dev"}
	tests := []struct {
		id      aws.Identity
		name    string
		want    string
		account aws.Account
	}{
		{name: "the identity row carries the ARN", account: aws.Account{ID: "123456789012", Identity: true}, id: id, want: "123456789012; " + id.ARN},
		{name: "an unknown ARN", account: aws.Account{ID: "123456789012", Identity: true}, id: aws.Identity{AccountID: "123456789012"}, want: "123456789012"},
		{name: "another account", account: aws.Account{ID: "210987654321"}, id: id, want: "210987654321"},
		{name: "the roles of the login", account: aws.Account{ID: "210987654321", Roles: []string{"ReadOnly", "Admin"}}, id: id, want: "210987654321; roles: ReadOnly, Admin"},
		{
			name: "the ARN, the roles and the role in use", id: id,
			account: aws.Account{ID: "123456789012", Identity: true, Roles: []string{"ReadOnly"}, Role: "ReadOnly"},
			want:    "123456789012; " + id.ARN + "; roles: ReadOnly; using ReadOnly",
		},
		{name: "a role in use without a list", account: aws.Account{ID: "210987654321", Role: "Admin"}, id: id, want: "210987654321; using Admin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := accountSummary(tt.account, tt.id); got != tt.want {
				t.Errorf("accountSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAWSStatusColors(t *testing.T) {
	d := newTestDashboard(t, Options{})
	tests := []struct {
		status string
		want   tcell.Color
	}{
		{status: "monitored", want: d.theme.StatusOK},
		{status: "monitored (first role Billing)", want: d.theme.StatusOK},
		{status: "following", want: d.theme.StatusOK},
		{status: "connected", want: d.theme.StatusOK},
		{status: "available", want: d.theme.StatusWarn},
		{status: "requires SSO", want: d.theme.StatusWarn},
		{status: "following (lagging)", want: d.theme.StatusWarn},
		{status: "not followed (cap)", want: d.theme.StatusWarn},
		{status: "-", want: d.theme.StatusWarn},
		{status: "connecting", want: d.theme.StatusWarn},
		{status: "error: SSO token expired - run aws sso login", want: d.theme.StatusError},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := d.awsStatusColor(tt.status); got != tt.want {
				t.Errorf("awsStatusColor(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

// The accounts of an SSO login, with the statuses the manager gives them: the
// roles and the role in use are part of the summary, and the status color follows
// the usual rule.
func TestAWSViewShowsSSOAccounts(t *testing.T) {
	d := newTestDashboard(t, Options{})
	d.awsViewMode = true
	const arn = "arn:aws:sts::123456789012:assumed-role/ReadOnly/dev"
	st := aws.AWSState{
		Conn:     aws.ConnConnected,
		Region:   "sa-east-1",
		Identity: aws.Identity{AccountID: "123456789012", ARN: arn},
		Accounts: []aws.Account{
			{ID: "123456789012", Name: "dev", Status: "monitored", Identity: true, Roles: []string{"ReadOnly", "Admin"}},
			{ID: "111111111111", Name: "audit", Status: "monitored (first role Billing)", Role: "Billing", Roles: []string{"Billing"}},
			{ID: "222222222222", Name: "beta", Status: "error: role Admin not listed for this account", Roles: []string{"ReadOnly"}},
			{ID: "333333333333", Name: "ops", Status: "available", Roles: []string{"ReadOnly"}},
			{ID: "444444444444", Name: "444444444444", Status: "requires SSO"},
		},
	}

	d.updateAWSTable(st)

	want := []struct {
		name, status, summary string
		color                 tcell.Color
	}{
		{name: "dev", status: "monitored", summary: "123456789012; " + arn + "; roles: ReadOnly, Admin", color: d.theme.StatusOK},
		{name: "audit", status: "monitored (first role Billing)", summary: "111111111111; roles: Billing; using Billing", color: d.theme.StatusOK},
		{name: "beta", status: "error: role Admin not listed for this account", summary: "222222222222; roles: ReadOnly", color: d.theme.StatusError},
		{name: "ops", status: "available", summary: "333333333333; roles: ReadOnly", color: d.theme.StatusWarn},
		{name: "444444444444", status: "requires SSO", summary: "444444444444", color: d.theme.StatusWarn},
	}
	if got := d.servicesTable.GetRowCount(); got != len(want)+1 {
		t.Fatalf("rows = %d, want the header and %d accounts", got, len(want))
	}
	for i, w := range want {
		row := i + 1
		name, status, summary := d.servicesTable.GetCell(row, 0), d.servicesTable.GetCell(row, 3), d.servicesTable.GetCell(row, 4)
		if name.Text != w.name || status.Text != w.status || summary.Text != w.summary {
			t.Errorf("row %d = (%q, %q, %q), want (%q, %q, %q)", row, name.Text, status.Text, summary.Text, w.name, w.status, w.summary)
		}
		if got := fgOf(status); got != w.color {
			t.Errorf("row %d: the status %q is in %v, want %v", row, w.status, got, w.color)
		}
	}
}

func TestAWSTitleCarriesTheDiscoveryProblem(t *testing.T) {
	connected := func(problem aws.Problem) aws.AWSState {
		return aws.AWSState{
			Region: "sa-east-1", Conn: aws.ConnConnected,
			Identity:       aws.Identity{AccountID: "123456789012"},
			DiscoveryError: problem,
		}
	}
	tests := []struct {
		name string
		want string
		st   aws.AWSState
	}{
		{
			name: "a problem shows its text only", st: connected(aws.Problem{Text: "SSO token expired", Hint: "run aws sso login"}),
			want: " AWS Cloud Resources (sa-east-1) connected: 123456789012; SSO token expired ",
		},
		{
			name: "a problem without a hint", st: connected(aws.Problem{Text: "throttled by AWS"}),
			want: " AWS Cloud Resources (sa-east-1) connected: 123456789012; throttled by AWS ",
		},
		{
			name: "a network problem shows its short form", st: connected(aws.Problem{Text: "AWS unreachable: timeout"}),
			want: " AWS Cloud Resources (sa-east-1) connected: 123456789012; AWS unreachable ",
		},
		{
			name: "a denied call shows its short form", st: connected(aws.Problem{Text: "access denied: ListAccounts"}),
			want: " AWS Cloud Resources (sa-east-1) connected: 123456789012; access denied ",
		},
		{
			name: "no problem", st: connected(aws.Problem{}),
			want: " AWS Cloud Resources (sa-east-1) connected: 123456789012 ",
		},
		{
			name: "the problem is escaped", st: connected(aws.Problem{Text: "ListAccounts failed [red]"}),
			want: " AWS Cloud Resources (sa-east-1) connected: 123456789012; ListAccounts failed [red[] ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := awsTitle(tt.st); got != tt.want {
				t.Errorf("awsTitle() = %q, want %q", got, tt.want)
			}
		})
	}

	// The same title reaches the table.
	d := newTestDashboard(t, Options{})
	d.awsViewMode = true
	st := connected(aws.Problem{Text: "SSO token expired", Hint: "run aws sso login"})
	st.Accounts = []aws.Account{{ID: "123456789012", Name: "123456789012", Status: aws.StatusMonitored, Identity: true}}
	d.updateAWSTable(st)
	if got, want := d.servicesTable.GetTitle(), " AWS Cloud Resources (sa-east-1) connected: 123456789012; SSO token expired "; got != want {
		t.Errorf("title of the table = %q, want %q", got, want)
	}
}

// tview draws the title of a box in width-2 columns and replaces the last one
// with an ellipsis when the title is longer, so at 80 columns a title has 78.
// Every title has to fit, with a region of nine characters and an account ID of
// twelve digits: it shows the short form of a problem (its text up to the first
// colon), never a hint and never the text of the SDK, and carries a discovery
// problem only next to a connected status. The full text and the hint show in the
// rows the problem affects or, with no account row, in the single explanatory row.
func TestFixedTitlesFitIn80Columns(t *testing.T) {
	const maxColumns = 78
	const sdkMarker = "a line of the SDK"
	sdkText := strings.Repeat(sdkMarker+" ", 12)[:200] // 200 characters of SDK text
	erroring := func(text, hint string) aws.AWSState {
		return aws.AWSState{Region: "sa-east-1", Conn: aws.ConnError, LastError: aws.Problem{Text: text, Hint: hint}}
	}
	connected := func(text, hint string) aws.AWSState {
		return aws.AWSState{
			Region: "sa-east-1", Conn: aws.ConnConnected,
			Identity:       aws.Identity{AccountID: "123456789012"},
			DiscoveryError: aws.Problem{Text: text, Hint: hint},
		}
	}
	// A failing identity keeps the discovery problem of an earlier pass.
	combined := func(connection, discovery aws.Problem) aws.AWSState {
		return aws.AWSState{Region: "sa-east-1", Conn: aws.ConnError, LastError: connection, DiscoveryError: discovery}
	}
	expired := aws.Problem{Text: "SSO token expired", Hint: "run aws sso login"}
	tests := []struct {
		name   string
		want   string // the exact title, when the test pins it
		absent string // a text the title must not carry
		st     aws.AWSState
	}{
		{name: "no credentials", st: erroring("no AWS credentials found", "configure the AWS CLI or set aws.enabled: false")},
		{name: "SSO token expired", st: erroring("SSO token expired", "run aws sso login")},
		{name: "keys expired or invalid", st: erroring("credentials expired or invalid", "log in again and re-export the keys")},
		{name: "credential_process failed", st: erroring("credential_process failed", "check the credential_process of the profile")},
		{name: "throttled", st: erroring("throttled by AWS", "")},
		{name: "access denied", st: erroring("access denied: GetCallerIdentity", "")},
		{name: "unreachable", st: erroring("AWS unreachable: DNS lookup failed", "")},
		{name: "profile not found", st: erroring(`AWS profile "default" not found`, "check AWS_PROFILE or aws.profile")},
		{name: "a failure of the SDK", st: erroring("GetCallerIdentity failed: "+sdkText, ""), absent: sdkMarker},

		{name: "SSO discovery problem with its hint", st: connected("SSO token expired", "run aws sso login")},
		{name: "SSO discovery throttled", st: connected("throttled by AWS", "")},
		{name: "SSO discovery timeout", st: connected("AWS unreachable: timeout", "")},
		{name: "SSO discovery DNS failure", st: connected("AWS unreachable: DNS lookup failed", "")},
		{name: "SSO discovery network error", st: connected("AWS unreachable: network error", "")},
		{name: "ListAccounts denied", st: connected("access denied: ListAccounts", "")},
		{name: "ListAccountRoles denied", st: connected("access denied: ListAccountRoles", "")},
		{name: "SSO discovery failure of the SDK", st: connected("ListAccounts failed: "+sdkText, ""), absent: sdkMarker},

		{
			name: "unreachable identity and an expired login", st: combined(aws.Problem{Text: "AWS unreachable: timeout"}, expired),
			want: " AWS Cloud Resources (sa-east-1) error: AWS unreachable ",
		},
		{
			name: "rejected keys and an expired login",
			st:   combined(aws.Problem{Text: "credentials expired or invalid", Hint: "log in again and re-export the keys"}, expired),
			want: " AWS Cloud Resources (sa-east-1) error: credentials expired or invalid ",
		},
		{
			name: "unreachable identity and an unreachable portal",
			st:   combined(aws.Problem{Text: "AWS unreachable: DNS lookup failed"}, aws.Problem{Text: "AWS unreachable: network error"}),
			want: " AWS Cloud Resources (sa-east-1) error: AWS unreachable ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title := awsTitle(tt.st)

			if n := utf8.RuneCountInString(title); n > maxColumns {
				t.Errorf("the title %q has %d columns, want at most %d", title, n, maxColumns)
			}
			if tt.want != "" && title != tt.want {
				t.Errorf("the title is %q, want %q", title, tt.want)
			}
			if tt.absent != "" && strings.Contains(title, tt.absent) {
				t.Errorf("the title %q carries the text of the SDK", title)
			}
			for _, hint := range []string{tt.st.LastError.Hint, tt.st.DiscoveryError.Hint} {
				if hint != "" && strings.Contains(title, hint) {
					t.Errorf("the title %q carries the hint %q", title, hint)
				}
			}
		})
	}
}

// Review Focus 3: an account named "Team [prod]" shows as written, not as a tag.
func TestAWSViewEscapesAccountNames(t *testing.T) {
	d := newTestDashboard(t, Options{})
	d.awsViewMode = true
	containers := sampleContainers()
	containers.AWS = aws.AWSState{
		Conn:     aws.ConnConnected,
		Region:   "sa-east-1",
		Identity: aws.Identity{AccountID: "123456789012"},
		Accounts: []aws.Account{
			{ID: "123456789012", Name: "Team [prod]", Status: aws.StatusMonitored, Identity: true, Roles: []string{"Role [x]"}},
		},
	}
	d.Update(containers)

	if got, want := d.servicesTable.GetCell(1, 0).Text, "Team [prod[]"; got != want {
		t.Errorf("name cell = %q, want %q", got, want)
	}
	if got, want := d.servicesTable.GetCell(1, 4).Text, "123456789012; roles: Role [x[]"; got != want {
		t.Errorf("summary cell = %q, want %q", got, want)
	}
	shot := drawDashboard(t, d, 160, 40)
	for _, text := range []string{"Team [prod]", "roles: Role [x]"} {
		if _, _, ok := shot.find(text); !ok {
			t.Errorf("%q is not on screen:\n%s", text, shot.text())
		}
	}
}

// The table is read from its first row. A tview table that was once shorter
// than its panel scrolls itself to its end when it grows, which hides the
// identity row as soon as the accounts do not fit; the single row of a pass that
// is still connecting is such a short table.
func TestAWSViewShowsTheTopOfALongList(t *testing.T) {
	const width, height = 160, 30 // the panel shows the header and two rows
	d := newTestDashboard(t, Options{})
	d.awsViewMode = true
	containers := sampleContainers()
	containers.AWS = aws.AWSState{Conn: aws.ConnConnecting}
	d.Update(containers)
	drawDashboard(t, d, width, height)

	accounts := []aws.Account{{ID: "123456789012", Name: "identity-account", Status: aws.StatusMonitored, Identity: true}}
	for i := 1; i <= 8; i++ {
		accounts = append(accounts, aws.Account{ID: fmt.Sprintf("2000000000%02d", i), Name: fmt.Sprintf("account-%02d", i), Status: aws.StatusAvailable})
	}
	containers.AWS = aws.AWSState{
		Conn:     aws.ConnConnected,
		Region:   "sa-east-1",
		Identity: aws.Identity{AccountID: "123456789012"},
		Accounts: accounts,
	}

	// The table is filled again at every refresh of the dashboard.
	for refresh := 1; refresh <= 2; refresh++ {
		d.Update(containers)
		shot := drawDashboard(t, d, width, height)

		if row, column := d.servicesTable.GetOffset(); row != 0 || column != 0 {
			t.Errorf("refresh %d: the table is scrolled to row %d, column %d, want its first row", refresh, row, column)
		}
		if _, _, ok := shot.find("identity-account"); !ok {
			t.Errorf("refresh %d: the identity row is not on screen:\n%s", refresh, shot.text())
		}
		visible := 0
		for _, a := range accounts {
			if _, _, ok := shot.find(a.Name); ok {
				visible++
			}
		}
		if visible == len(accounts) {
			t.Fatalf("refresh %d: all %d accounts are on screen, so the list fits and the test proves nothing:\n%s", refresh, visible, shot.text())
		}
	}
}

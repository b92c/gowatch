package ui

import (
	"testing"
	"time"

	"github.com/b92c/gowatch/internal/aws"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// noCredentials is the problem the AWS manager reports when the SDK finds no
// credential provider.
var noCredentials = aws.Problem{Text: "no AWS credentials found", Hint: "configure the AWS CLI or set aws.enabled: false"}

func TestAWSViewTitle(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		st      aws.AWSState
		hasRows bool
	}{
		{name: "the zero state", st: aws.AWSState{}, want: " AWS Cloud Resources (us-east-1) not configured "},
		{name: "not configured", st: aws.AWSState{Region: "eu-west-1", Conn: aws.ConnNotConfigured}, want: " AWS Cloud Resources (eu-west-1) not configured "},
		{name: "disabled", st: aws.AWSState{Conn: aws.ConnDisabled}, want: " AWS Cloud Resources (us-east-1) disabled "},
		{name: "connecting", st: aws.AWSState{Conn: aws.ConnConnecting}, want: " AWS Cloud Resources (us-east-1) connecting "},
		{
			name: "connected", hasRows: true,
			st:   aws.AWSState{Region: "sa-east-1", Conn: aws.ConnConnected, Identity: aws.Identity{AccountID: "123456789012"}},
			want: " AWS Cloud Resources (sa-east-1) connected: 123456789012 ",
		},
		{
			name: "an error with no row to carry the hint",
			st:   aws.AWSState{Conn: aws.ConnError, LastError: noCredentials},
			want: " AWS Cloud Resources (us-east-1) error: no AWS credentials found - configure the AWS CLI or set aws.enabled: false ",
		},
		{
			name: "an error with a row that carries the hint", hasRows: true,
			st:   aws.AWSState{Conn: aws.ConnError, LastError: noCredentials},
			want: " AWS Cloud Resources (us-east-1) error: no AWS credentials found ",
		},
		{
			name: "an error without a hint",
			st:   aws.AWSState{Conn: aws.ConnError, LastError: aws.Problem{Text: "AWS unreachable: timeout"}},
			want: " AWS Cloud Resources (us-east-1) error: AWS unreachable: timeout ",
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
			if got := awsTitle(tt.st, tt.hasRows); got != tt.want {
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

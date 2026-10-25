package ui

import (
	"strings"

	"github.com/b92c/gowatch/internal/aws"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Texts and sizes of the AWS table. The explanatory texts fill the single row
// that stands in for the accounts when there is nothing to list.
const (
	explainNotConfigured = "AWS not configured: set up the AWS CLI or set aws.enabled: true"
	explainDisabled      = "AWS integration is disabled (aws.enabled: false)"
	explainConnecting    = "Connecting to AWS..."
	explainNoRows        = "No AWS resources"
	awsTypeAccount       = "Account"
	awsNameMaxWidth      = 60
)

// awsFallbackRegion is the region the title shows for a state that has none:
// the region the AWS settings fall back to.
const awsFallbackRegion = "us-east-1"

// awsTableHeaders are the six columns of the AWS table.
var awsTableHeaders = []string{"Resource Name", "Type", "Region", "Status", "Metrics Summary", "Last Updated"}

// awsRegion is the region the view shows for st.
func awsRegion(st aws.AWSState) string {
	if st.Region == "" {
		return awsFallbackRegion
	}
	return st.Region
}

// awsTitle is the title of the services table in the AWS view: the region the
// SDK resolved, then the connection status. The title shows the short form of a
// problem (its text up to the first colon) and never a hint, so that it fits an
// 80-column terminal: the full text and the hint show in the Status column of the
// rows the problem affects ("error: <text> - <hint>") or, when there is no account
// row, in the single explanatory row (awsExplanation). A problem with the SSO
// discovery follows a connected status, so a connected identity with an expired
// SSO login says why the accounts are old; its rows are the aws.accounts entries
// still waiting for credentials, and with none of them the short form here is
// all that shows. When the identity itself fails, the title shows that problem
// only, and the rows keep showing the discovery one.
func awsTitle(st aws.AWSState) string {
	status := awsStatus(st)
	if st.Conn == aws.ConnConnected && st.DiscoveryError.Text != "" {
		status += "; " + st.DiscoveryError.Short()
	}
	return " AWS Cloud Resources (" + tview.Escape(awsRegion(st)) + ") " + tview.Escape(status) + " "
}

// awsStatus is the status part of the title. A failed connection shows the short
// form of its problem and never its hint.
func awsStatus(st aws.AWSState) string {
	switch st.Conn {
	case aws.ConnDisabled:
		return "disabled"
	case aws.ConnConnecting:
		return "connecting"
	case aws.ConnConnected:
		return "connected: " + st.Identity.AccountID
	case aws.ConnError:
		return "error: " + st.LastError.Short()
	}
	return "not configured" // ConnNotConfigured and the zero state
}

// awsExplanation is the text of the single row shown when there are no
// accounts to list.
func awsExplanation(st aws.AWSState) string {
	switch st.Conn {
	case aws.ConnDisabled:
		return explainDisabled
	case aws.ConnConnecting:
		return explainConnecting
	case aws.ConnError:
		return st.LastError.Message()
	case aws.ConnConnected:
		return explainNoRows
	}
	return explainNotConfigured // ConnNotConfigured and the zero state
}

// accountSummary is the Metrics Summary cell of an account: its ID; for the
// account of the active credentials, the ARN of the caller; the roles the SSO
// login can use in it; and the role it is monitored with.
func accountSummary(a aws.Account, id aws.Identity) string {
	summary := a.ID
	if a.Identity && id.ARN != "" {
		summary += "; " + id.ARN
	}
	if len(a.Roles) > 0 {
		summary += "; roles: " + strings.Join(a.Roles, ", ")
	}
	if a.Role != "" {
		summary += "; using " + a.Role
	}
	return summary
}

// awsStatusColor colors a status: red for an error, green for a healthy one
// and yellow for anything that needs attention or is only a possibility.
func (d *Dashboard) awsStatusColor(status string) tcell.Color {
	switch {
	case strings.HasPrefix(status, "error"):
		return d.theme.StatusError
	case status == aws.StatusMonitored,
		strings.HasPrefix(status, aws.StatusMonitored+" ("),
		status == aws.StatusFollowing,
		status == string(aws.ConnConnected):
		return d.theme.StatusOK
	}
	return d.theme.StatusWarn
}

// updateAWSTable fills the services table with the AWS view: the title, the
// header and one row per account, or a single muted row that says why there is
// none. Every text that comes from AWS goes through tview.Escape, because the
// table reads [text] as a color tag.
func (d *Dashboard) updateAWSTable(st aws.AWSState) {
	t := d.theme
	d.servicesTable.Clear()
	// A tview table that was once shorter than its panel keeps itself scrolled to
	// its end when it grows, so without this the identity row would leave the
	// screen once the accounts do not fit.
	d.servicesTable.ScrollToBeginning()
	d.servicesTable.SetTitle(awsTitle(st))

	for col, header := range awsTableHeaders {
		d.servicesTable.SetCell(0, col, tview.NewTableCell(header).
			SetTextColor(t.Accent).
			SetAlign(tview.AlignCenter).
			SetSelectable(false))
	}

	if len(st.Accounts) == 0 {
		d.servicesTable.SetCell(1, 0, tview.NewTableCell(tview.Escape(awsExplanation(st))).
			SetTextColor(t.Muted).
			SetAlign(tview.AlignLeft).
			SetSelectable(false))
		return
	}

	region := awsRegion(st)
	for row, a := range st.Accounts {
		cells := []struct {
			text  string
			color tcell.Color
		}{
			{text: a.Name, color: t.Text},
			{text: awsTypeAccount, color: t.Info},
			{text: region, color: t.Muted},
			{text: a.Status, color: d.awsStatusColor(a.Status)},
			{text: accountSummary(a, st.Identity), color: t.Text},
			{text: a.UpdatedAt.Format("15:04:05"), color: t.Muted},
		}
		for col, c := range cells {
			cell := tview.NewTableCell(tview.Escape(c.text)).
				SetTextColor(c.color).
				SetAlign(tview.AlignLeft)
			if col == 0 {
				cell.SetMaxWidth(awsNameMaxWidth)
			}
			d.servicesTable.SetCell(row+1, col, cell)
		}
	}
}

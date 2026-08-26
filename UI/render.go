package UI

import (
	"fmt"
	"strings"

	"EZDeploy/core"
)

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	var body string
	switch m.view {
	case viewProjects:
		body = m.renderProjects()
	case viewProject:
		body = m.renderProject()
	case viewOverview:
		body = m.renderOverview()
	case viewReleases:
		body = m.renderReleases()
	case viewLogs:
		body = m.renderLogs()
	case viewNetwork:
		body = m.renderNetwork()
	case viewSystem:
		body = m.renderSystem()
	case viewInstall:
		body = m.renderInstall()
	case viewConfirm:
		body = m.renderConfirm()
	}
	return signatureStyle.Render(signature) + "\n" + subtitleStyle.Render("project deployment dashboard") + "\n\n" + body + m.renderMessage()
}

func (m Model) renderProjects() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Projects") + "\n")
	if len(m.projects) == 0 {
		b.WriteString("No projects registered. Start with: sudo ezdeploy deploy <repository>\n\n")
	}
	for index, name := range m.projects {
		project := m.registry[name]
		line := fmt.Sprintf("%-20s %-10s %-28s %2d service(s)  %s", name, project.Status, project.Domain, len(project.ManagedServices(name)), short(project.Revision))
		b.WriteString(m.item(index, line))
	}
	b.WriteString(m.item(len(m.projects), "System"))
	b.WriteString(footerStyle.Render("↑/↓ navigate · enter select · q quit"))
	return b.String()
}

func (m Model) renderProject() string {
	project := m.project()
	var b strings.Builder
	b.WriteString(headerStyle.Render(m.selected) + "\n")
	b.WriteString(fmt.Sprintf("%s · %s · %d service(s) · revision %s\n\n", project.Status, project.Domain, len(project.ManagedServices(m.selected)), short(project.Revision)))
	for index, action := range projectActions {
		b.WriteString(m.item(index, action))
	}
	b.WriteString(footerStyle.Render("↑/↓ navigate · enter select · esc projects"))
	return b.String()
}

func (m Model) renderOverview() string {
	project := m.project()
	var b strings.Builder
	b.WriteString(headerStyle.Render(m.selected+" — Overview") + "\n")
	b.WriteString(fmt.Sprintf("Repository: %s\nPath:       %s\nDomain:     %s\nBranch:     %s\nRevision:   %s\nStatus:     %s\nReleases:   %d/20\n", project.RepoURL, project.Path, project.Domain, project.Branch, project.Revision, project.Status, len(project.Releases)))
	for _, service := range project.ManagedServices(m.selected) {
		b.WriteString(fmt.Sprintf("\nService %s\n  runtime %s · root %s · port %d · status %s\n", service.Name, service.Runtime, service.Root, service.Port, service.Status))
	}
	b.WriteString(footerStyle.Render("esc back"))
	return b.String()
}

func (m Model) renderReleases() string {
	releases := m.project().Releases
	var b strings.Builder
	b.WriteString(headerStyle.Render(m.selected+" — Releases and rollback") + "\n")
	if len(releases) == 0 {
		b.WriteString("No releases recorded yet. The current revision is preserved on the next successful deployment.\n\n")
	}
	for index := len(releases) - 1; index >= 0; index-- {
		release := releases[index]
		b.WriteString(m.item(len(releases)-1-index, fmt.Sprintf("%-31s %-8s %s  %s", release.ID, release.Operation, release.DeployedAt.Local().Format("2006-01-02 15:04"), short(release.Revision))))
	}
	b.WriteString(m.item(len(releases), "Back"))
	b.WriteString(footerStyle.Render("enter select release · esc back"))
	return b.String()
}

func (m Model) renderLogs() string {
	managed := m.project().ManagedServices(m.selected)
	var b strings.Builder
	b.WriteString(headerStyle.Render(m.selected+" — Logs") + "\n")
	b.WriteString(m.item(0, "Deployment activity"))
	for index, service := range managed {
		b.WriteString(m.item(index+1, "Runtime · "+service.Name))
	}
	b.WriteString(m.item(len(managed)+1, "Back"))
	b.WriteString(footerStyle.Render("enter last 100 lines · f follow · esc back"))
	return b.String()
}

func (m Model) renderNetwork() string {
	report := m.network
	var detail string
	if !report.MetadataAvailable {
		detail = "EC2 IMDSv2 metadata unavailable. Automatic IP detection cannot run; no third-party IP service was contacted."
	} else if report.Match {
		detail = "Healthy: " + report.Hostname + " resolves to " + report.PublicIPv4 + "."
	} else {
		addresses := "no IPv4 A record"
		if len(report.Addresses) > 0 {
			addresses = strings.Join(report.Addresses, ", ")
		}
		detail = fmt.Sprintf("Mismatch: %s resolves to %s. Update %s A to %s; no redeploy required.", report.Hostname, addresses, report.Record, report.PublicIPv4)
	}
	return headerStyle.Render(m.selected+" — Network and DNS") + "\n" + detail + "\n\nElastic IP is the stable solution. Reboot retains the public IP; stop/start normally changes it.\n" + footerStyle.Render("r refresh · esc back")
}

func (m Model) renderSystem() string {
	os := core.GetOS()
	return headerStyle.Render("System") + "\n" + fmt.Sprintf("Host: %s (%s)\n\n", os.System, os.ID) + m.item(0, "Install components") + m.item(1, "Live metrics") + m.item(2, "Back") + footerStyle.Render("↑/↓ navigate · enter select · esc projects")
}

func (m Model) renderInstall() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Install components") + "\n")
	for index, item := range installItems {
		box := "[ ] "
		if m.chosen[index] {
			box = "[x] "
		}
		b.WriteString(m.item(index, box+item))
	}
	b.WriteString(footerStyle.Render("↑/↓ navigate · space select · enter install · esc back"))
	return b.String()
}

func (m Model) renderConfirm() string {
	if m.confirm == "rollback" {
		return headerStyle.Render("Confirm rollback") + "\n" + fmt.Sprintf("Restore %s to release %s?\n\nCode and server configuration will roll back. Database schema and data will NOT; that remains your responsibility.\n", m.selected, m.releaseID) + footerStyle.Render("enter confirm · esc cancel")
	}
	return headerStyle.Render("Confirm removal") + "\nRemove " + m.selected + " from the registry? Running services and files are not deleted.\n" + footerStyle.Render("enter confirm · esc cancel")
}

func (m Model) item(index int, label string) string {
	if m.cursor == index {
		return selectedStyle.Render("> "+label) + "\n"
	}
	return "  " + label + "\n"
}

func (m Model) renderMessage() string {
	if m.lastMsg == "" {
		return ""
	}
	if m.lastOK {
		return "\n" + resultOKStyle.Render("✓ "+m.lastMsg) + "\n"
	}
	return "\n" + resultErrStyle.Render("✗ "+m.lastMsg) + "\n"
}

func short(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const taskName = "KadduDesk Server"

func autostartEnabled() bool {
	return hiddenCmd("schtasks.exe", "/Query", "/TN", taskName).Run() == nil
}

func setAutostart(on bool) error {
	if !on {
		out, err := hiddenCmd("schtasks.exe", "/Delete", "/F", "/TN", taskName).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	exe, _ := os.Executable()
	xml := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Description>Starts KadduDesk remote desktop server at logon</Description></RegistrationInfo>
  <Triggers><LogonTrigger><Enabled>true</Enabled><Delay>PT5S</Delay></LogonTrigger></Triggers>
  <Principals><Principal id="Author"><LogonType>InteractiveToken</LogonType><RunLevel>HighestAvailable</RunLevel></Principal></Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>4</Priority>
  </Settings>
  <Actions Context="Author"><Exec><Command>` + xmlEscape(exe) + `</Command><Arguments>-minimized</Arguments></Exec></Actions>
</Task>`
	tmp := filepath.Join(os.TempDir(), "kaddudesk-task.xml")
	// schtasks wants UTF-16 for the declared encoding
	u := []byte{0xFF, 0xFE}
	for _, r := range xml {
		u = append(u, byte(r), byte(r>>8))
	}
	if err := os.WriteFile(tmp, u, 0o600); err != nil {
		return err
	}
	defer os.Remove(tmp)
	out, err := hiddenCmd("schtasks.exe", "/Create", "/F", "/TN", taskName, "/XML", tmp).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

const fwRule = "KadduDesk Server"

func addFirewallRule(public bool) error {
	exe, _ := os.Executable()
	return addFirewallRuleFor(exe, public)
}

func addFirewallRuleFor(exe string, public bool) error {
	profile := "private,domain"
	if public {
		profile = "any"
	}
	hiddenCmd("netsh", "advfirewall", "firewall", "delete", "rule", "name="+fwRule).Run()
	out, err := hiddenCmd("netsh", "advfirewall", "firewall", "add", "rule", "name="+fwRule, "dir=in",
		"action=allow", "program="+exe, "enable=yes", "profile="+profile).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	logf("firewall rule added (profile=%s)", profile)
	return nil
}

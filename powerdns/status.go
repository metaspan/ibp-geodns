package powerdns

import (
	"encoding/json"
	"fmt"
	"ibp-geodns/config"
	"ibp-geodns/matrixbot"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

var previousStatus = make(map[string]map[string]map[string]bool)
var mu sync.RWMutex

func updateMemberStatus() {
	for result := range resultsChannel {
		// log.Printf("Received result: %s", result)

		results := strings.Split(result, "\n")
		for _, res := range results {
			if strings.TrimSpace(res) == "" {
				continue
			}

			var siteStatus config.SiteResults
			var endpointStatus config.EndpointResults

			if err := json.Unmarshal([]byte(res), &siteStatus); err == nil && siteStatus.ResultType == "site" {
				updateSiteStatus(siteStatus)
			} else if err := json.Unmarshal([]byte(res), &endpointStatus); err == nil && endpointStatus.ResultType == "endpoint" {
				updateEndpointStatus(endpointStatus)
			} else {
				log.Printf("Error parsing result: %v", err)
			}
		}
	}
}

func updateSiteStatus(status config.SiteResults) {
	// log.Printf("Updating site status: %+v", status)

	for memberName, checks := range status.Members {
		for checkName, result := range checks {
			member, memberExists := getMember(memberName)
			if !memberExists {
				continue
			}

			if previousStatus["site"] == nil {
				previousStatus["site"] = make(map[string]map[string]bool)
			}

			if previousStatus["site"][memberName] == nil {
				previousStatus["site"][memberName] = make(map[string]bool)
			}

			if previousStatus["site"][memberName][checkName] != result.Success {
				// log.Printf("Status change detected for site member %s check %s: %v -> %v", memberName, checkName, previousStatus["site"][memberName][checkName], result.Success)

				if result.Success {
					if member.Results[checkName].OfflineTS.IsZero() {
						member.Results[checkName] = Result{Success: true}
						previousStatus["site"][memberName][checkName] = result.Success
					} else if time.Since(member.Results[checkName].OfflineTS).Seconds() <= float64(configData.MinimumOfflineTime) {
						continue
					}

					if !member.Results[checkName].OfflineTS.IsZero() && time.Since(member.Results[checkName].OfflineTS).Seconds() >= float64(configData.MinimumOfflineTime) {
						member.Results[checkName] = Result{Success: true}
						previousStatus["site"][memberName][checkName] = result.Success

						if !member.Override {
							sendMatrixMessage(fmt.Sprintf("<b>Adding member</b> <i>%s</i> <b>to all rotations</b><br><i><b>Server:</b> %s</i><br><i><b>Check %s:</b> false -> true</i><BR><b>Result Data:</b> %v", memberName, configData.ServerName, checkName, result.CheckData))
							logStatusChange("Site Status Change", memberName, checkName, false, true, result.CheckData)
						}
					}
				} else {
					member.Results[checkName] = Result{
						Success:   false,
						OfflineTS: time.Now(),
					}

					previousStatus["site"][memberName][checkName] = result.Success
					if !member.Override {
						sendMatrixMessage(fmt.Sprintf("<b>Removing member</b> <i>%s</i> <b>from all rotations</b><br><i><b>Server:</b> %s</i><br><i><b>Check %s:</b> true -> false</i><BR><b>Result Data:</b> %v", memberName, configData.ServerName, checkName, result.CheckData))
						logStatusChange("Site Status Change", memberName, checkName, true, false, result.CheckData)
					}
				}

				for i := range powerDNSConfigs {
					if _, memberExists := powerDNSConfigs[i].Members[memberName]; memberExists {
						powerDNSConfigs[i].Members[memberName] = member
					}
				}
			} else {
				// If new and previous check results are false increase OfflineTS counter to extend offline time.
				if !result.Success && !member.Results[checkName].Success {
					member.Results[checkName] = Result{
						Success:   false,
						OfflineTS: time.Now(),
					}

					for i := range powerDNSConfigs {
						if _, memberExists := powerDNSConfigs[i].Members[memberName]; memberExists {
							powerDNSConfigs[i].Members[memberName] = member
						}
					}
				}
			}
		}
	}
}

func updateEndpointStatus(status config.EndpointResults) {
	// log.Printf("Updating endpoint status: %+v", status)

	for endpointURL, members := range status.Endpoint {
		for memberName, checks := range members {
			for checkName, result := range checks {

				if previousStatus[endpointURL] == nil {
					previousStatus[endpointURL] = make(map[string]map[string]bool)
				}

				if previousStatus[endpointURL][memberName] == nil {
					previousStatus[endpointURL][memberName] = make(map[string]bool)
				}

				member, memberExists := getMember(memberName)
				if !memberExists {
					continue
				}

				if previousStatus[endpointURL][memberName][checkName] != result.Success {
					// log.Printf("Status change detected for endpoint member %s check %s: %v -> %v", memberName, checkName, previousStatus[endpointURL][memberName][checkName], result.Success)

					if result.Success {
						if member.Results[checkName].OfflineTS.IsZero() {
							member.Results[checkName] = Result{Success: true}
							previousStatus[endpointURL][memberName][checkName] = result.Success
						} else if time.Since(member.Results[checkName].OfflineTS).Seconds() <= float64(configData.MinimumOfflineTime) {
							continue
						}

						if !member.Results[checkName].OfflineTS.IsZero() && time.Since(member.Results[checkName].OfflineTS).Seconds() >= float64(configData.MinimumOfflineTime) {
							member.Results[checkName] = Result{Success: true}
							previousStatus[endpointURL][memberName][checkName] = result.Success

							if !member.Override {
								sendMatrixMessage(fmt.Sprintf("<b>Adding member</b> <i>%s</i> <b>to endpoint</b> <i>%s</i><br><i><b>Server:</b> %s</i><br><i><b>Check %s:</b> false -> true</i><BR><b>Result Data:</b> %v", memberName, endpointURL, configData.ServerName, checkName, result.CheckData))
								logStatusChange("EndPoint Status Change", memberName, checkName, false, true, result.CheckData)
							}
						}

					} else {
						member.Results[checkName] = Result{
							Success:   false,
							OfflineTS: time.Now(),
						}

						previousStatus[endpointURL][memberName][checkName] = result.Success

						if !member.Override {
							sendMatrixMessage(fmt.Sprintf("<b>Removing member</b> <i>%s</i> <b>from endpoint</b> <i>%s</i><br><i><b>Server:</b> %s</i><br><i><b>Check %s:</b> true -> false</i><BR><b>Result Data:</b> %v", memberName, endpointURL, configData.ServerName, checkName, result.CheckData))
							logStatusChange("EndPoint Status Change", memberName, checkName, true, false, result.CheckData)
						}
					}

					for j := range powerDNSConfigs {
						if powerDNSConfigs[j].Domain == endpointURL {
							if _, memberExists := powerDNSConfigs[j].Members[memberName]; memberExists {
								powerDNSConfigs[j].Members[memberName] = member
							}
						}
					}
				} else {
					// If new and previous check results are false increase OfflineTS counter to extend offline time.
					if !result.Success && !member.Results[checkName].Success {
						member.Results[checkName] = Result{
							Success:   false,
							OfflineTS: time.Now(),
						}

						for j := range powerDNSConfigs {
							if powerDNSConfigs[j].Domain == endpointURL {
								if _, memberExists := powerDNSConfigs[j].Members[memberName]; memberExists {
									powerDNSConfigs[j].Members[memberName] = member
								}
							}
						}
					}
				}
			}
		}
	}
}

func getMember(memberName string) (Member, bool) {
	for _, config := range powerDNSConfigs {
		if member, exists := config.Members[memberName]; exists {
			if member.Results == nil {
				member.Results = make(map[string]Result)
			}
			return member, true
		}
	}
	return Member{}, false
}

func sendMatrixMessage(message string) {
	bot, err := matrixbot.NewMatrixBot(configData.Matrix.HomeServerURL, configData.Matrix.Username, configData.Matrix.Password, configData.Matrix.RoomID)
	if err != nil {
		log.Printf("Error initializing Matrix bot: %v", err)
	} else {
		go bot.SendMessage(message)
	}
}

func logStatusChange(changeType, memberName, checkName string, prevSuccess, newSuccess bool, resultData interface{}) {
	log.Printf("%s: Server %s - member %s - Check %s: %v -> %v - Result Data: %v", changeType, configData.ServerName, memberName, checkName, prevSuccess, newSuccess, resultData)
}

func statusOutput(w http.ResponseWriter, r *http.Request) {
	sort.SliceStable(powerDNSConfigs, func(i, j int) bool {
		return powerDNSConfigs[i].Domain < powerDNSConfigs[j].Domain
	})

	for i := range powerDNSConfigs {
		memberNames := make([]string, 0, len(powerDNSConfigs[i].Members))
		for memberName := range powerDNSConfigs[i].Members {
			memberNames = append(memberNames, memberName)
		}
		sort.Strings(memberNames)

		sortedMembers := make(map[string]Member)
		for _, memberName := range memberNames {
			sortedMembers[memberName] = powerDNSConfigs[i].Members[memberName]
		}
		powerDNSConfigs[i].Members = sortedMembers
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<html><head><title> %s Status Page </title>", configData.ServerName))
	sb.WriteString(`<style>
		body { font-family: Arial, sans-serif; font-size: 12px; }
		h2, h3 { color: #2c3e50; margin: 0; padding: 0; }
		.container { display: flex; }
		.column { width: 50%; padding: 10px; box-sizing: border-box; }
		table { width: 100%; border-collapse: collapse; margin-bottom: 10px; }
		th, td { border: 1px solid #ddd; padding: 4px; text-align: left; font-size: 12px; }
		th { background-color: #f2f2f2; }
		ul { margin: 0; padding-left: 20px; }
		li { margin: 0; padding: 0; }
	</style></head><body>`)

	sb.WriteString(fmt.Sprintf("<h2>Server: %s</h2>", configData.ServerName))
	sb.WriteString("<div class='container'>")

	// Divide the domains into two columns
	half := (len(powerDNSConfigs) + 1) / 2 // +1 to ensure we handle odd number of configs
	for col := 0; col < 2; col++ {
		sb.WriteString("<div class='column'>")
		for i := col * half; i < (col+1)*half && i < len(powerDNSConfigs); i++ {
			config := powerDNSConfigs[i]
			sb.WriteString(fmt.Sprintf("<h3>Domain: %s</h3>", config.Domain))
			sb.WriteString("<table><tr><th>Member</th><th>Override</th><th>IPv4</th><th>IPv6</th><th>Lat</th><th>Lon</th><th>Results</th></tr>")
			memberNames := make([]string, 0, len(config.Members))
			for memberName := range config.Members {
				memberNames = append(memberNames, memberName)
			}
			sort.Strings(memberNames)
			for _, memberName := range memberNames {
				member := config.Members[memberName]
				sb.WriteString("<tr>")
				sb.WriteString(fmt.Sprintf("<td>%s</td><td>%t</td><td>%s</td><td>%s</td><td>%.2f</td><td>%.2f</td><td><ul>", memberName, member.Override, member.IPv4, member.IPv6, member.Latitude, member.Longitude))
				for checkName, result := range member.Results {
					sb.WriteString(fmt.Sprintf("<li>%s: %v", checkName, result.Success))
					if !result.OfflineTS.IsZero() {
						sb.WriteString(fmt.Sprintf(", %s", result.OfflineTS.Format("2006-01-02 15:04")))
					}
					sb.WriteString("</li>")
				}
				sb.WriteString("</ul></td></tr>")
			}
			sb.WriteString("</table>")
		}
		sb.WriteString("</div>")
	}

	sb.WriteString("</div></body></html>")

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(sb.String()))
}

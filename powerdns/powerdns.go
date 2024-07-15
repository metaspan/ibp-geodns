package powerdns

import (
	"log"
	"net/http"
	"strings"
)

var (
	powerDNSConfigs []DNS
	resultsChannel  chan string
	staticEntries   map[string][]Record
	topLevelDomains map[string]bool
)

func Init(configs []DNS, resultsCh chan string, geoIPDBPath string, staticEntriesURL string) {
	err := InitGeoIP(geoIPDBPath)
	if err != nil {
		log.Printf("Failed to initialize GeoIP database: %v", err)
	}

	err = loadStaticEntries(staticEntriesURL)
	if err != nil {
		log.Printf("Failed to load static entries: %v", err)
	}

	go startStaticEntriesUpdater(staticEntriesURL)

	powerDNSConfigs = configs
	resultsChannel = resultsCh

	topLevelDomains = make(map[string]bool)
	for _, config := range configs {
		parts := strings.Split(config.Domain, ".")
		if len(parts) > 1 {
			topLevelDomain := strings.Join(parts[len(parts)-2:], ".")
			topLevelDomains[topLevelDomain] = true
		}
	}

	go updateMemberStatus()

	http.HandleFunc("/dns", dnsHandler)
	log.Println("Starting PowerDNS server on :8080")
	go http.ListenAndServe(":8080", nil)
}

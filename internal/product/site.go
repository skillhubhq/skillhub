// Package product 定义官方发行身份；自建 Hub 不能通过自报名称获得官方标识。
package product

import (
	_ "embed"
	"encoding/json"
)

//go:embed site.json
var siteJSON []byte

type Identity struct {
	Name        string `json:"name"`
	OfficialHub string `json:"officialHub"`
	Repository  string `json:"repository"`
}

var Site = loadSite()

func loadSite() Identity {
	var site Identity
	if err := json.Unmarshal(siteJSON, &site); err != nil {
		panic("invalid product configuration")
	}
	return site
}

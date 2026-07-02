// Command clickvault serves the ClickHouse database secrets engine plugin
// for HashiCorp Vault.
package main

import (
	dbplugin "github.com/hashicorp/vault/sdk/database/dbplugin/v5"

	"github.com/emiliano-go/clickvault/internal/clickvault"
)

func main() {
	dbplugin.ServeMultiplex(clickvault.New)
}

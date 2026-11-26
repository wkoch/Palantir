// Package ui embute a interface web compilada por scripts/construir.sh.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Arquivos devolve os arquivos da interface. Sem a compilação, o diretório
// contém apenas o marcador do Git.
func Arquivos() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	return sub
}

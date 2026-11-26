package proxy

import "os"

func replaceWithDir(caminho string) error {
	if err := os.Remove(caminho); err != nil {
		return err
	}
	return os.Mkdir(caminho, 0o700)
}

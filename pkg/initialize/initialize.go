package initialize

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/ayushjn20/dunner/internal"
	"github.com/ayushjn20/dunner/internal/logger"
)

// InitProject generates a dunner task file with default template
func InitProject(filename string) error {
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		if err != nil {
			return err
		}
		return fmt.Errorf("%s already exists", filename)
	}
	logger.Log.Infof("Generating %s file", filename)
	return ioutil.WriteFile(filename, []byte(internal.DefaultTaskFileContents), internal.DefaultTaskFilePermission)
}

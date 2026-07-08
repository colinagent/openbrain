package chat

import (
	"errors"
	"os"
	"strings"
)

func isThreadNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	message := strings.TrimSpace(err.Error())
	if message == os.ErrNotExist.Error() {
		return true
	}
	return strings.HasSuffix(message, ": "+os.ErrNotExist.Error())
}

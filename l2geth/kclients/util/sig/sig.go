package sig

import (
	"fmt"
	"github.com/ethereum-optimism/optimism/l2geth/log"
	"os"
	"sync"
	"syscall"
)

var once sync.Once

func Int(reason string) {
	log.Info("### DEBUG ### Trying to shut down process by SIGINT", "reason", reason)
	once.Do(func() {
		err := syscall.Kill(os.Getpid(), syscall.SIGINT)
		if err != nil {
			fmt.Println("Error sending signal:", err)
		}
	})
}

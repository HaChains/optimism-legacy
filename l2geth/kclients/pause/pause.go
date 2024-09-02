package pause

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum-optimism/optimism/l2geth/ethclient"
	"github.com/ethereum-optimism/optimism/l2geth/kclients/util/env"
	"github.com/ethereum-optimism/optimism/l2geth/kclients/util/sig"
	"github.com/ethereum-optimism/optimism/l2geth/log"
	"github.com/go-redis/redis/v8"
)

type Result struct {
	BlockNumber int64 `json:"blockNumber"`
}

var rdb *redis.Client

var (
	addrs      = []string{}
	masterName string
	password   string
	db         int
)

func redisBlockNumber() int64 {
	// offset用于测试
	//var offset int64 = 2493000

	if rdb == nil {
		rdb = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:    masterName,
			SentinelAddrs: addrs,
			Password:      password,
			DB:            db,
		})
	}
	ctx := context.Background()
	str, err := rdb.HGet(ctx, "chain_latest:timeline", "Optimism").Result()
	if err != nil {
		log.Error("### DEBUG ### redis HGet err", "err", err)
		return -1
	}
	var r Result
	err = json.Unmarshal([]byte(str), &r)
	if err != nil {
		log.Error("### DEBUG ### json unmarshall err", "err", err)
		return -1
	}
	return r.BlockNumber - pc.testOffset
}

var pc pauseControl

type pauseControl struct {
	ctx         context.Context
	cli         *ethclient.Client
	started     bool
	allowOffset int64
	testOffset  int64
	//l2Height    int64
	nextHeight  int64
	redisHeight int64
	lock        sync.RWMutex
}

func Start(ctx context.Context) {
	if Started() {
		return
	}
	addrs = env.LoadEnvStrings(env.EnvETLAddrs)
	if len(addrs) == 0 {
		sig.Int("empty etl redis endpoint list")
		return
	}
	masterName = env.LoadEnvString(env.EnvETLMasterName)
	if masterName == "" {
		sig.Int("invalid etl redis master name")
		return
	}
	password = env.LoadEnvStringMute(env.EnvETLPassword)
	if password == "" {
		sig.Int("invalid etl redis password")
		return
	}
	db = env.LoadEnvInt(env.EnvETLDB)
	if db == env.WrongInt {
		return
	}
	pc.allowOffset = env.LoadEnvInt64(env.EnvETLAllowBehind)
	if pc.allowOffset == env.WrongInt {
		return
	}
	pc.testOffset = env.LoadEnvInt64(env.EnvTestOffset)
	if pc.testOffset == env.WrongInt {
		return
	}
	pc.ctx = ctx
	go pc.updateLoop()
	log.Info("### DEBUG ### pause control service started")
	pc.started = true
}

func (c *pauseControl) updateLoop() {
	c.updateBlockHeight()
	for {
		select {
		case <-time.After(time.Second):
			c.updateBlockHeight()
		case <-c.ctx.Done():
			log.Info("### DEBUG ### pauseControl updateLoop stopped")
			return
		}
	}
}

func (c *pauseControl) updateBlockHeight() {
	redisHeight := redisBlockNumber()
	//var err error
	//if c.cli == nil {
	//	c.cli, err = ethclient.Dial("http://127.0.0.1:8545/")
	//	if err != nil {
	//		log.Error(fmt.Sprintf("### DEBUG ### retry later, cli dial err: %s", err))
	//		return
	//	}
	//}
	//c.lock.Lock()
	//header, err := c.cli.HeaderByNumber(context.Background(), nil)
	//if err != nil {
	//	if strings.Contains(err.Error(), "dial tcp 127.0.0.1:8545: connect: connection refused") {
	//		log.Info("### DEBUG ### retry later, local rpc service not start")
	//		c.lock.Unlock()
	//		return
	//	}
	//	log.Error("### DEBUG ### retry later", "err", err)
	//	c.lock.Unlock()
	//	return
	//}
	//l2Height := header.Number.Int64()
	//c.lock.Unlock()

	c.lock.Lock()
	if redisHeight != -1 {
		c.redisHeight = redisHeight
	}
	//c.l2Height = l2Height
	c.lock.Unlock()
	//log.Debug(fmt.Sprintf("### DEBUG ### update block height, l2:%d, redis: %d", l2Height, redisHeight))
}

func Started() bool {
	return pc.started
}

func RedisBehind(l2Num int64) bool {
	return pc.redisBehind(l2Num)
}

func PauseIfBehind(tag string) (shutdown bool) {
	return pc.pauseIfBehind(tag)
}
func (c *pauseControl) redisBehind(l2Num int64) bool {
	if !c.started {
		fmt.Println("### DEBUG ### [pauseControl.redisBehind] service is not started")
	}
	c.lock.RLock()
	if l2Num == 0 {
		l2Num = c.nextHeight
	} else {
		c.nextHeight = l2Num
	}
	var pause = l2Num-c.redisHeight >= c.allowOffset
	c.lock.RUnlock()
	log.Info(fmt.Sprintf("### DEBUG ### l2Height(%d)-redisHeight(%d) = %d >= allowOffset(%d): %v",
		l2Num, c.redisHeight, l2Num-c.redisHeight, c.allowOffset, pause))
	//log.Info("### DEBUG ###", "l2 height from rpc", c.l2Height)
	return pause
}

func (c *pauseControl) pauseIfBehind(tag string) (shutdown bool) {
	stopCh := make(chan struct{})
	for {
		select {
		case <-time.After(1 * time.Second):
			//log.Debug(fmt.Sprintf("### DEBUG ### check redis block height [%s]", tag))
			if !c.redisBehind(0) {
				close(stopCh)
			}
		case <-stopCh:
			log.Info("### DEBUG ### stop pause", "tag", tag)
			return false
		case <-c.ctx.Done():
			if rdb != nil {
				rdb.Close()
			}
			if pc.cli != nil {
				pc.cli.Close()
			}
			log.Info("### DEBUG ### Pause Control exit", "tag", tag)
			return true
		}
	}
}

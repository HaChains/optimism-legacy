package pause

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum-optimism/optimism/l2geth/ethclient"
	"github.com/ethereum-optimism/optimism/l2geth/log"
	"github.com/go-redis/redis/v8"
)

type Result struct {
	BlockNumber int64 `json:"blockNumber"`
}

var rdb *redis.Client

func redisBlockNumber() int64 {
	// offset用于测试
	//var offset int64 = 2493000
	var offset int64 = 0
	op := redis.FailoverOptions{
		MasterName:    "mymaster",
		SentinelAddrs: []string{"192.168.224.126:26379", "192.168.224.127:26379", "192.168.224.129:26379"},
		Password:      "1qaz@WSX2022",
		DB:            0,
	}
	if rdb == nil {
		rdb = redis.NewFailoverClient(&op)
	}
	ctx := context.Background()
	get, _ := rdb.HMGet(ctx, "chain_latest:timeline", "Optimism").Result()
	var r Result
	if len(get) < 1 {
		return -1
	}
	str, ok := (get[0]).(string)
	if !ok {
		log.Info("### DEBUG ### can not convert to string", "var", get[0])
		return -1
	}
	err := json.Unmarshal([]byte(str), &r)
	if err != nil {
		log.Error("### DEBUG ### json unmarshall err", "err", err)
		return -1
	}
	return r.BlockNumber - offset
}

var pc pauseControl

type pauseControl struct {
	ctx         context.Context
	cli         *ethclient.Client
	started     bool
	allowOffset int64
	l2Height    int64
	redisHeight int64
	lock        sync.RWMutex
}

func Start(ctx context.Context) {
	if pc.started {
		return
	}
	pc.ctx = ctx
	pc.started = true
	pc.allowOffset = 128
	go pc.updateLoop()
}

func (c *pauseControl) updateLoop() {
	for {
		c.updateBlockHeight()
		<-time.After(time.Second)
	}
}

func (c *pauseControl) updateBlockHeight() {
	redisHeight := redisBlockNumber()
	var err error
	if c.cli == nil {
		c.cli, err = ethclient.Dial("http://127.0.0.1:8545/")
		if err != nil {
			log.Error(fmt.Sprintf("### DEBUG ### retry later cli dial err: %s", err))
			return
		}
	}
	c.lock.Lock()
	header, err := c.cli.HeaderByNumber(context.Background(), nil)
	if err != nil {
		log.Error("### DEBUG ### retry later", "err", err)
		c.lock.Unlock()
		return
	}
	l2Height := header.Number.Int64()
	c.lock.Unlock()

	c.lock.Lock()
	if redisHeight != -1 {
		c.redisHeight = redisHeight
	}
	c.l2Height = l2Height
	c.lock.Unlock()
	log.Debug(fmt.Sprintf("### DEBUG ### update block height, l2:%d, redis: %d", l2Height, redisHeight))
}

func RedisBehind(l2Num int64) bool {
	return pc.redisBehind(l2Num)
}

func PauseIfBehind(tag string) {
	pc.pauseIfBehind(tag)
}
func (c *pauseControl) redisBehind(l2Num int64) bool {
	c.lock.RLock()
	if l2Num == 0 {
		l2Num = c.l2Height
	}
	var pause = l2Num-c.redisHeight > c.allowOffset
	c.lock.RUnlock()
	// log sampling
	if l2Num%2 == 0 {
		log.Info(fmt.Sprintf("### DEBUG ### l2Height(%d)-redisHeight(%d) = %d > allowOffset(%d): %v",
			l2Num, c.redisHeight, l2Num-c.redisHeight, c.allowOffset, pause))
		log.Info("### DEBUG ###", "l2 height from rpc", c.l2Height)
	}
	return pause
}

func (c *pauseControl) pauseIfBehind(tag string) {
	stopCh := make(chan struct{})
	for {
		select {
		case <-time.After(1 * time.Second):
			//log.Debug(fmt.Sprintf("### DEBUG ### check redis block height [%s]", tag))
			if !c.redisBehind(0) {
				close(stopCh)
			}
		case <-stopCh:
			log.Debug(fmt.Sprintf("### DEBUG ### stop pause [%s]", tag))
			return
		case <-c.ctx.Done():
			log.Info("### DEBUG ###", "Pause Control exit")
			return
		}
	}
}

package adminhook

import "time"

// 测试里去掉退避等待。
func (c *Client) FastRetry() { c.backoff = func(int) time.Duration { return time.Millisecond } }

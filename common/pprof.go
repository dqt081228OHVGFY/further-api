package common

import (
	"context"
	"fmt"
	"os"
	"runtime/pprof"
	"time"

	"github.com/shirou/gopsutil/cpu"
)

// Monitor 定时监控cpu使用率，超过阈值输出pprof文件
func Monitor() {
	MonitorContext(BackgroundContext())
}

func MonitorContext(ctx context.Context) {
	for ctx.Err() == nil {
		percent, err := cpu.PercentWithContext(ctx, time.Second, false)
		if err != nil {
			if ctx.Err() == nil {
				SysError("CPU monitor failed: " + err.Error())
			}
		} else if len(percent) > 0 && percent[0] > 80 {
			fmt.Println("cpu usage too high")
			// write pprof file
			if _, err := os.Stat("./pprof"); os.IsNotExist(err) {
				err := os.Mkdir("./pprof", 0700)
				if err != nil {
					SysLog("创建pprof文件夹失败 " + err.Error())
					continue
				}
			}
			f, err := os.Create("./pprof/" + fmt.Sprintf("cpu-%s.pprof", time.Now().Format("20060102150405")))
			if err != nil {
				SysLog("创建pprof文件失败 " + err.Error())
				continue
			}
			err = pprof.StartCPUProfile(f)
			if err != nil {
				SysLog("启动pprof失败 " + err.Error())
				f.Close()
				continue
			}
			timer := time.NewTimer(10 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
			pprof.StopCPUProfile()
			f.Close()
		}
		timer := time.NewTimer(30 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

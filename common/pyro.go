package common

import (
	"runtime"
	"sync"

	"github.com/grafana/pyroscope-go"
)

var pyroscopeMu sync.Mutex
var pyroscopeProfiler *pyroscope.Profiler

func StartPyroScope() error {
	pyroscopeMu.Lock()
	defer pyroscopeMu.Unlock()
	if pyroscopeProfiler != nil {
		return nil
	}

	pyroscopeUrl := GetEnvOrDefaultString("PYROSCOPE_URL", "")
	if pyroscopeUrl == "" {
		return nil
	}

	pyroscopeAppName := GetEnvOrDefaultString("PYROSCOPE_APP_NAME", "further-api")
	pyroscopeBasicAuthUser := GetEnvOrDefaultString("PYROSCOPE_BASIC_AUTH_USER", "")
	pyroscopeBasicAuthPassword := GetEnvOrDefaultString("PYROSCOPE_BASIC_AUTH_PASSWORD", "")
	pyroscopeHostname := GetEnvOrDefaultString("HOSTNAME", "further-api")

	mutexRate := GetEnvOrDefault("PYROSCOPE_MUTEX_RATE", 5)
	blockRate := GetEnvOrDefault("PYROSCOPE_BLOCK_RATE", 5)

	runtime.SetMutexProfileFraction(mutexRate)
	runtime.SetBlockProfileRate(blockRate)

	profiler, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: pyroscopeAppName,

		ServerAddress:     pyroscopeUrl,
		BasicAuthUser:     pyroscopeBasicAuthUser,
		BasicAuthPassword: pyroscopeBasicAuthPassword,

		Logger: nil,

		Tags: map[string]string{"hostname": pyroscopeHostname},

		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,

			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
	if err != nil {
		return err
	}
	pyroscopeProfiler = profiler
	return nil
}

func StopPyroScope() error {
	pyroscopeMu.Lock()
	defer pyroscopeMu.Unlock()
	if pyroscopeProfiler == nil {
		return nil
	}
	profiler := pyroscopeProfiler
	pyroscopeProfiler = nil
	return profiler.Stop()
}

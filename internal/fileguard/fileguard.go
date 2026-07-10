package fileguard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

var ErrStabilityTimeout = errors.New("file stability timeout")

type StabilityOptions struct {
	Timeout       time.Duration
	Interval      time.Duration
	StableSamples int
}

type StabilityResult struct {
	Path        string
	Size        int64
	ModTime     time.Time
	CheckedAt   time.Time
	SampleCount int
}

func WaitUntilStable(ctx context.Context, path string, opt StabilityOptions) (StabilityResult, error) {
	opt = withDefaults(opt)
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithTimeout(ctx, opt.Timeout)
	defer cancel()

	ticker := time.NewTicker(opt.Interval)
	defer ticker.Stop()

	var lastSize int64 = -1
	var lastModTime time.Time
	stableSamples := 0
	sampleCount := 0
	var lastErr error

	for {
		result, err := sample(path)
		sampleCount++
		if err != nil {
			lastErr = err
			stableSamples = 0
		} else {
			if result.Size == lastSize && result.ModTime.Equal(lastModTime) {
				stableSamples++
			} else {
				stableSamples = 1
				lastSize = result.Size
				lastModTime = result.ModTime
			}

			if stableSamples >= opt.StableSamples {
				result.SampleCount = sampleCount
				return result, nil
			}
		}

		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				if lastErr != nil {
					return StabilityResult{Path: path, SampleCount: sampleCount}, fmt.Errorf("%w: %w", ErrStabilityTimeout, lastErr)
				}
				return StabilityResult{Path: path, SampleCount: sampleCount}, ErrStabilityTimeout
			}
			return StabilityResult{Path: path, SampleCount: sampleCount}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func withDefaults(opt StabilityOptions) StabilityOptions {
	if opt.Timeout <= 0 {
		opt.Timeout = 120 * time.Second
	}
	if opt.Interval <= 0 {
		opt.Interval = time.Second
	}
	if opt.StableSamples <= 0 {
		opt.StableSamples = 3
	}
	return opt
}

func sample(path string) (StabilityResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return StabilityResult{Path: path, CheckedAt: time.Now()}, err
	}
	if info.IsDir() {
		return StabilityResult{Path: path, CheckedAt: time.Now()}, fmt.Errorf("%s is a directory", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return StabilityResult{Path: path, Size: info.Size(), ModTime: info.ModTime(), CheckedAt: time.Now()}, err
	}
	if err := f.Close(); err != nil {
		return StabilityResult{Path: path, Size: info.Size(), ModTime: info.ModTime(), CheckedAt: time.Now()}, err
	}

	return StabilityResult{
		Path:      path,
		Size:      info.Size(),
		ModTime:   info.ModTime(),
		CheckedAt: time.Now(),
	}, nil
}

package ffmpeg

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"time"
)

// Progress is one update parsed from ffmpeg's -progress stream. Percent is
// best-effort: it needs the source duration and stays 0 for images (which
// have none) until Done.
type Progress struct {
	Percent   float64       // 0..100
	OutTime   time.Duration // position reached in the output
	Frame     int64
	FPS       float64
	Speed     float64 // realtime multiple; 2.5 means 2.5x
	Bitrate   string
	TotalSize int64 // bytes written so far
	Done      bool  // true on the final block (progress=end)
}

// parseProgress consumes ffmpeg's `key=value` -progress stream from r and
// calls fn once per block (blocks end with a `progress=` line). durationSec
// scales Percent; pass 0 when unknown. It returns when r is exhausted.
func parseProgress(r io.Reader, durationSec float64, fn func(Progress)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)

	var p Progress
	for sc.Scan() {
		key, val, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok {
			continue
		}
		switch key {
		case "frame":
			p.Frame, _ = strconv.ParseInt(val, 10, 64)
		case "fps":
			p.FPS, _ = strconv.ParseFloat(val, 64)
		case "bitrate":
			p.Bitrate = val
		case "total_size":
			p.TotalSize, _ = strconv.ParseInt(val, 10, 64)
		case "out_time_us", "out_time_ms":
			// Both keys carry microseconds in ffmpeg's output (a known
			// quirk of the "_ms" name).
			if us, err := strconv.ParseInt(val, 10, 64); err == nil && us > 0 {
				p.OutTime = time.Duration(us) * time.Microsecond
			}
		case "speed":
			p.Speed = parseSpeed(val)
		case "progress":
			if durationSec > 0 {
				p.Percent = clampF(p.OutTime.Seconds()/durationSec*100, 0, 100)
			}
			if val == "end" {
				p.Percent = 100
				p.Done = true
			}
			fn(p)
			p.Done = false // carry cumulative fields into the next block
		}
	}
}

func parseSpeed(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "x"))
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

package ffmpeg

import (
	"strings"
	"testing"
	"time"
)

const sampleProgress = `frame=30
fps=30.00
bitrate=1000.0kbits/s
total_size=131072
out_time_us=1000000
out_time_ms=1000000
out_time=00:00:01.000000
speed=2.0x
progress=continue
frame=60
fps=30.00
bitrate=1000.0kbits/s
total_size=262144
out_time_us=2000000
out_time_ms=2000000
out_time=00:00:02.000000
speed=2.5x
progress=continue
frame=90
fps=30.00
total_size=393216
out_time_us=3000000
out_time_ms=3000000
speed=N/A
progress=end
`

func TestParseProgress(t *testing.T) {
	var got []Progress
	parseProgress(strings.NewReader(sampleProgress), 3.0, func(p Progress) {
		got = append(got, p)
	})

	if len(got) != 3 {
		t.Fatalf("got %d blocks, want 3", len(got))
	}

	if got[0].Frame != 30 || got[0].OutTime != time.Second || got[0].Speed != 2.0 {
		t.Errorf("block 0 = %+v", got[0])
	}
	if pct := got[0].Percent; pct < 33 || pct > 34 {
		t.Errorf("block 0 percent = %.2f, want ~33.3", pct)
	}
	if got[1].Percent < 66 || got[1].Percent > 67 {
		t.Errorf("block 1 percent = %.2f, want ~66.7", got[1].Percent)
	}
	if got[1].TotalSize != 262144 {
		t.Errorf("block 1 total_size = %d", got[1].TotalSize)
	}

	last := got[2]
	if !last.Done || last.Percent != 100 {
		t.Errorf("final block = %+v, want Done && Percent==100", last)
	}
	if last.Speed != 0 {
		t.Errorf("speed N/A should parse to 0, got %v", last.Speed)
	}
}

func TestParseProgressNoDuration(t *testing.T) {
	var last Progress
	parseProgress(strings.NewReader(sampleProgress), 0, func(p Progress) { last = p })
	// Without a duration, Percent stays 0 until the end block forces 100.
	if last.Percent != 100 || !last.Done {
		t.Errorf("final = %+v, want 100 + Done", last)
	}
}

func TestParseSpeed(t *testing.T) {
	for in, want := range map[string]float64{
		"2.5x": 2.5, "1x": 1, " 0.75x ": 0.75, "N/A": 0, "": 0, "x": 0,
	} {
		if got := parseSpeed(in); got != want {
			t.Errorf("parseSpeed(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestWatermarkArgsGetProgressFlags(t *testing.T) {
	e := testEngine()
	// buildArgs never sees OnProgress; the flags are inserted by Watermark,
	// so drive the same insertion here.
	base, _ := e.buildArgs(Request{SourcePath: "in.mp4", OutputPath: "out.mp4", Media: MediaVideo, Overlay: OverlayLogo})
	withProg := append(base[:1:1], append([]string{"-progress", "pipe:1", "-nostats"}, base[1:]...)...)

	if withProg[0] != "-y" || withProg[1] != "-progress" || withProg[2] != "pipe:1" || withProg[3] != "-nostats" {
		t.Errorf("progress flags not inserted after -y: %v", withProg[:5])
	}
	if base[0] != "-y" || base[1] == "-progress" {
		t.Errorf("buildArgs output was mutated: %v", base[:3])
	}
}

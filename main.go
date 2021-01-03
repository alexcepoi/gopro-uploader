package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

const VideoExt = ".mp4"

type VideoResolution struct {
	Width     int
	Height    int
	Codec     string
	FrameRate float64
}

type Chapter struct {
	FileName   string
	CreateTime time.Time
	Duration   time.Duration
	Resolution VideoResolution
}

type Video struct {
	Title    string
	Path     string
	Chapters []Chapter
}

// Returns the list of videos present in directory.
func listRenderedVideos(dirPath string) ([]string, error) {
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var results []string
	for _, file := range files {
		if strings.HasSuffix(strings.ToLower(file.Name()), VideoExt) {
			results = append(results, strings.TrimSuffix(file.Name(), VideoExt))
		}
	}
	return results, nil
}

// Parses a string like 60/1 or 15360/256 to determine actual frame rate.
func parseFrameRate(spec string) (float64, error) {
	parts := strings.Split(spec, "/")
	if len(parts) != 2 {
		return 0.0, fmt.Errorf("Error parsing frame rate: %s", spec)
	}
	x, err := strconv.ParseInt(parts[0], 0, 64)
	if err != nil {
		return 0.0, fmt.Errorf("Error parsing frame rate: %s", spec)
	}
	y, err := strconv.ParseInt(parts[1], 0, 64)
	if err != nil {
		return 0.0, fmt.Errorf("Error parsing frame rate: %s", spec)
	}
	return float64(x) / float64(y), nil
}

// Creates a chapter object from file metadata.
func fetchChapter(dirPath, fileName string) (*Chapter, error) {
	cmd := exec.Command("ffprobe", "-v", "error", path.Join(dirPath, fileName),
		"-print_format", "json", "-show_format", "-show_streams")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var data struct {
		Format struct {
			Duration string
			Tags     struct {
				Creation_time string
			}
		}
		Streams []struct {
			Coded_width    int
			Coded_height   int
			Codec_name     string
			Avg_frame_rate string
		}
	}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		return nil, err
	}
	duration, err := time.ParseDuration(data.Format.Duration + "s")
	if err != nil {
		return nil, err
	}
	createTime, err := time.Parse(time.RFC3339Nano, data.Format.Tags.Creation_time)
	if err != nil {
		return nil, err
	}
	frame_rate, err := parseFrameRate(data.Streams[0].Avg_frame_rate)
	if err != nil {
		return nil, err
	}

	return &Chapter{
		FileName:   fileName,
		Duration:   duration,
		CreateTime: createTime,
		Resolution: VideoResolution{
			Width:     data.Streams[0].Coded_width,
			Height:    data.Streams[0].Coded_height,
			Codec:     data.Streams[0].Codec_name,
			FrameRate: frame_rate,
		}}, nil
}

// Returns all chapters from a directory (non-recursive).
// TODO(alexcepoi): Add support for timelapses.
// ffmpeg -framerate 60 -pattern_type glob -i '*.JPG' output.mp4
func getChapters(dirPath string) ([]Chapter, error) {
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var results []Chapter
	for _, file := range files {
		if !file.IsDir() &&
			strings.HasSuffix(strings.ToLower(file.Name()), VideoExt) &&
			!strings.HasPrefix(file.Name(), ".") {
			chapter, err := fetchChapter(dirPath, file.Name())
			if err != nil {
				return nil, err
			}
			results = append(results, *chapter)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreateTime.Before(results[j].CreateTime)
	})
	return results, nil
}

// Verify if two chapters are compatible with ffmpeg concat demuxer.
func canUseConcatDemuxer(x, y Chapter) bool {
	if x.Resolution.Width != y.Resolution.Width {
		return false
	}
	if x.Resolution.Height != y.Resolution.Height {
		return false
	}
	if x.Resolution.Codec != y.Resolution.Codec {
		return false
	}
	return true
}

// Splits a video into multiple ones so that ffmpeg concat demuxer can be aplied
// to all chapters in each video.
func splitVideo(video Video) []Video {
	var chapter_batches [][]Chapter
	for ix, chapter := range video.Chapters {
		if ix == 0 || !canUseConcatDemuxer(chapter, video.Chapters[ix-1]) {
			chapter_batches = append(chapter_batches, []Chapter{})
		}
		last_batch := &chapter_batches[len(chapter_batches)-1]
		*last_batch = append(*last_batch, chapter)
	}

	if len(chapter_batches) == 1 {
		return []Video{video}
	}

	var results []Video
	for ix, batch := range chapter_batches {
		results = append(results, Video{
			Title:    fmt.Sprintf("%s pt %d", video.Title, ix+1),
			Path:     video.Path,
			Chapters: batch,
		})
	}
	return results
}

// Generates a title for the video based on path.
func generateVideoTitle(dirPath, rootPath, prefix string) string {
	var parts []string
	var part string

	dirPath = path.Clean(dirPath)
	rootPath = path.Clean(rootPath)
	for dirPath != "" && dirPath != "/" && dirPath != rootPath {
		dirPath, part = path.Split(dirPath)
		dirPath = path.Clean(dirPath)
		parts = append([]string{part}, parts...)
	}

	return fmt.Sprintf("[%s] %s", prefix, strings.Join(parts, " # "))
}

// Generates a description for the video based on its chapters.
func generateVideoDescription(chapters []Chapter) string {
	var lines []string
	var startTime time.Duration
	for _, chapter := range chapters {
		lines = append(lines,
			fmt.Sprintf("%s | %s [%dx%d @ %06.2f ~ %s]",
				fmtDurationForYouTube(startTime),
				chapter.FileName,
				chapter.Resolution.Width,
				chapter.Resolution.Height,
				chapter.Resolution.FrameRate,
				chapter.CreateTime.Format(time.RFC1123)))
		startTime += chapter.Duration
	}
	return strings.Join(lines, "\n")
}

// Write metadata file including chapter information.
func writeMetadata(video Video, outputFile string) error {
	chapterStartTimeMs := func(i int, chapters []Chapter) int64 {
		start := int64(0)
		for _, chapter := range chapters[:i] {
			start += chapter.Duration.Milliseconds()
		}
		return start
	}
	chapterEndTimeMs := func(i int, chapters []Chapter) int64 {
		return chapterStartTimeMs(i, chapters) + chapters[i].Duration.Milliseconds()
	}

	var tmpl = template.Must(template.New("metadata").Funcs(template.FuncMap{
		"startTimeMs": chapterStartTimeMs,
		"endTimeMs":   chapterEndTimeMs,
	}).Parse(`;FFMETADATA1
title={{.Title}}
{{ range $i, $ch := .Chapters }}
[CHAPTER]
TIMEBASE=1/1000
START={{ startTimeMs $i $.Chapters }}
END={{ endTimeMs $i $.Chapters  }}
title={{ $ch.FileName }}
{{ end  }}`))
	f, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	return tmpl.Execute(f, video)
}

// Renders a video concatenating its chapters.
func renderVideo(video Video, outputDir string) error {
	tmpDir, err := ioutil.TempDir("", "gopro-uploader")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	var inputLines []string
	for _, chapter := range video.Chapters {
		inputLines = append(inputLines,
			fmt.Sprintf("file '%s'", path.Join(video.Path, chapter.FileName)))
	}

	inputFname := filepath.Join(tmpDir, "input.txt")
	if err := ioutil.WriteFile(
		inputFname, []byte(strings.Join(inputLines, "\n")), os.ModePerm); err != nil {
		return err
	}
	metadataFname := filepath.Join(tmpDir, "chapters.txt")
	err = writeMetadata(video, metadataFname)
	if err != nil {
		return err
	}
	outputFname := filepath.Join(outputDir, video.Title+VideoExt)
	log.Printf(">>> Rendering %s", outputFname)
	cmd := exec.Command("ffmpeg", "-v", "warning",
		"-f", "concat", "-safe", "0",
		"-i", inputFname,
		"-i", metadataFname,
		"-map_metadata", "1",
		"-c", "copy", outputFname,
		"-y", "-stats")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	checkDependencies("ffprobe", "ffmpeg")

	inputDir := flag.String("input_dir", "", "Directory to traverse for video files.")
	outputDir := flag.String("output_dir", "", "Directory in which to output rendered video files.")
	prefix := flag.String("prefix", "", "Prefix to use in all video titles.")
	dryRun := flag.Bool("dry_run", false, "If true, does not attempt to render videos.")
	flag.Parse()
	if *inputDir == "" {
		log.Fatalf("--inputDir cannot be empty")
	}
	if *outputDir == "" {
		log.Fatalf("--outputDir cannot be empty")
	}
	if *prefix == "" {
		log.Fatalf("--prefix cannot be empty")
	}

	err := os.Mkdir(*outputDir, os.ModePerm)
	if err != nil && !os.IsExist(err) {
		log.Fatal(err)
	}

	titles, err := listRenderedVideos(*outputDir)
	if err != nil {
		log.Fatal(err)
	}

	err = filepath.Walk(*inputDir, func(dirPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}

		chapters, err := getChapters(dirPath)
		if err != nil {
			return err
		}
		if len(chapters) == 0 {
			return nil
		}

		videoTitle := generateVideoTitle(dirPath, *inputDir, *prefix)
		for _, video := range splitVideo(Video{Title: videoTitle, Path: dirPath, Chapters: chapters}) {
			log.Printf("=== %s\n%v", video.Title, generateVideoDescription(video.Chapters))
			if contains(titles, video.Title) {
				log.Printf(">>> Already rendered.. skipping..")
				continue
			}
			if *dryRun {
				continue
			}
			return renderVideo(video, *outputDir)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

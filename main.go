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
	"time"

	"golang.org/x/net/context"
	"google.golang.org/api/youtube/v3"
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

func fetchChapter(dirPath, fileName string) (*Chapter, error) {
	cmd := exec.Command("ffprobe", "-v", "error", path.Join(dirPath, fileName), "-print_format", "json", "-show_format", "-show_streams")
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

	return &Chapter{FileName: fileName, Duration: duration, CreateTime: createTime, Resolution: VideoResolution{
		Width:     data.Streams[0].Coded_width,
		Height:    data.Streams[0].Coded_height,
		Codec:     data.Streams[0].Codec_name,
		FrameRate: frame_rate,
	}}, nil
}

func getChapters(dirPath string) ([]Chapter, error) {
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var results []Chapter
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), VideoExt) && !strings.HasPrefix(file.Name(), ".") {
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

func generateChapterDescriptions(chapters []Chapter) []string {
	var lines []string
	var startTime time.Duration
	for _, chapter := range chapters {
		lines = append(lines, fmt.Sprintf("%s | %s [%dx%d @ %06.2f ~ %s]", fmtDuration(startTime), chapter.FileName, chapter.Resolution.Width, chapter.Resolution.Height, chapter.Resolution.FrameRate, chapter.CreateTime.Format(time.RFC1123)))
		startTime += chapter.Duration
	}
	return lines
}

func uploadVideo(video Video, uploadFn func(path string) error) error {
	dir, err := ioutil.TempDir("", "gopro-uploader")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	var inputLines []string
	for _, chapter := range video.Chapters {
		inputLines = append(inputLines, fmt.Sprintf("file '%s'", path.Join(video.Path, chapter.FileName)))
	}

	inputFname := filepath.Join(dir, "input.txt")
	if err := ioutil.WriteFile(inputFname, []byte(strings.Join(inputLines, "\n")), 0644); err != nil {
		return err
	}
	outputFname := filepath.Join(dir, "output"+VideoExt)
	log.Printf(">>> Rendering %s", outputFname)
	cmd := exec.Command("ffmpeg", "-v", "warning", "-f", "concat", "-safe", "0", "-i", inputFname, "-c", "copy", outputFname, "-y", "-stats")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return err
	}
	return uploadFn(outputFname)
}

func main() {
	checkDependencies("ffprobe", "ffmpeg")

	dir := flag.String("dir", "", "Directory to traverse for video files.")
	prefix := flag.String("prefix", "", "Prefix to use in all video titles.")
	playlistId := flag.String("playlist_id", "", "Playlist to put videos in.")
	dryRun := flag.Bool("dry_run", false, "If true, does not attempt to upload videos.")
	flag.Parse()
	if *dir == "" {
		log.Fatalf("--dir cannot be empty")
	}
	if *prefix == "" {
		log.Fatalf("--prefix cannot be empty")
	}
	if *playlistId == "" {
		log.Fatalf("--playlist_id cannot be empty")
	}

	ctx := context.Background()
	client_opts, err := newGoogleOAuth2Client(ctx, youtube.YoutubeScope)
	if err != nil {
		log.Fatal(err)
	}
	service, err := youtube.NewService(ctx, client_opts)
	if err != nil {
		log.Fatalf("Error creating YouTube client: %v", err)
	}
	titles, err := listVideoTitlesInPlaylist(service, *playlistId)
	if err != nil {
		log.Fatal(err)
	}

	err = filepath.Walk(*dir, func(dirPath string, info os.FileInfo, err error) error {
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

		videoTitle := generateVideoTitle(dirPath, *dir, *prefix)
		for _, video := range splitVideo(Video{Title: videoTitle, Path: dirPath, Chapters: chapters}) {
			chapterDescriptions := generateChapterDescriptions(video.Chapters)
			log.Printf("> %s", video.Title)
			for _, chapterDesc := range chapterDescriptions {
				log.Printf("*** %s", chapterDesc)
			}
			if contains(titles, video.Title) {
				log.Printf(">>> Already uploaded.. skipping..")
				continue
			}
			if *dryRun {
				continue
			}
			err := uploadVideo(
				video,
				func(path string) error {
					log.Printf(">>> Uploading from %v", path)
					id, err := uploadVideoToYoutube(service, YoutubeVideo{
						Title:         video.Title,
						Description:   strings.Join(chapterDescriptions, "\n"),
						CategoryId:    "17",
						PrivacyStatus: "unlisted",
						Path:          path,
						CreateTime:    video.Chapters[0].CreateTime,
					})
					if err != nil {
						return err
					}
					log.Printf(">>> Upload successful: https://youtu.be/%s", id)
					return addVideoToPlaylist(service, *playlistId, id)
				})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

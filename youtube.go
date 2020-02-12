package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"google.golang.org/api/youtube/v3"
)

type YoutubeVideo struct {
	Title         string
	Description   string
	CategoryId    string
	PrivacyStatus string
	Path          string
	CreateTime    time.Time
}

func shouldWaitForQuota(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), "quotaExceeded") || strings.Contains(err.Error(), "dailyLimitExceeded") {
		log.Printf("--- Waiting for API quota: %s", err)
		time.Sleep(time.Hour)
		return true
	}
	return false
}

func listVideoTitlesInPlaylist(service *youtube.Service, playlistId string) ([]string, error) {
	var titles []string
	nextPageToken := ""
	for {
		call := service.PlaylistItems.List("snippet")
		call = call.PlaylistId(playlistId)
		call = call.PageToken(nextPageToken)
		response, err := call.Do()
		for shouldWaitForQuota(err) {
			response, err = call.Do()
		}
		if err != nil {
			return nil, fmt.Errorf("Error listing playlist items: %s", err)
		}
		for _, video := range response.Items {
			titles = append(titles, video.Snippet.Title)
		}
		nextPageToken = response.NextPageToken
		if nextPageToken == "" {
			break
		}
	}
	return titles, nil
}

func addVideoToPlaylist(service *youtube.Service, playlistId string, videoId string) error {
	call := service.PlaylistItems.Insert("snippet", &youtube.PlaylistItem{
		Snippet: &youtube.PlaylistItemSnippet{
			PlaylistId: playlistId,
			ResourceId: &youtube.ResourceId{Kind: "youtube#video", VideoId: videoId},
		}})
	_, err := call.Do()
	for shouldWaitForQuota(err) {
		_, err = call.Do()
	}
	if err != nil {
		return fmt.Errorf("Error adding video to playlist: %s", err)
	}
	return nil
}

func uploadVideoToYoutube(service *youtube.Service, video YoutubeVideo) (string, error) {
	upload := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       video.Title,
			Description: video.Description,
			CategoryId:  video.CategoryId,
		},
		Status: &youtube.VideoStatus{PrivacyStatus: video.PrivacyStatus},
		RecordingDetails: &youtube.VideoRecordingDetails{
			RecordingDate: video.CreateTime.Format("2006-01-02T15:04:05.000Z"),
		},
	}
	upload.Snippet.Tags = []string{"GoPro"}
	call := service.Videos.Insert("snippet,status,recordingDetails", upload)

	file, err := os.Open(video.Path)
	if err != nil {
		return "", fmt.Errorf("Error opening %v: %v", video.Path, err)
	}
	defer file.Close()
	call = call.Media(file)

	response, err := call.Do()
	for shouldWaitForQuota(err) {
		response, err = call.Do()
	}
	if err != nil {
		return "", fmt.Errorf("Error uploading video: %s", err)
	}
	fmt.Printf("Upload successful! https://youtu.be/%s\n", response.Id)
	return response.Id, nil
}

func fmtDuration(d time.Duration) string {
	num_hours := int64(d.Hours())
	num_minutes := int64(d.Minutes())
	num_seconds := int64(d.Seconds())
	return fmt.Sprintf("%01d:%02d:%02d", num_hours, num_minutes-60*num_hours, num_seconds-60*num_minutes)
}

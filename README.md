# gopro-uploader

Command line tool which takes a directory hierarchy of GoPro chapter files,
groups and merges them into videos and uploads them to YouTube.

## Requirements

You'll need to have the following installed:

- golang
- ffmpeg


Installation instructions depend on platform. If you use [homebrew](https://brew.sh/),
that's:

```sh
brew install go ffmpeg
```

Before you can use this tool, you'll have to register it as an app with Google.
The process is a bit involved:

* Generate OAuth 2.0 credentials in the API console per [instructions](https://developers.google.com/youtube/registering_an_application). You might need to create a new project for this purpose. Download the
    credential file and store it as `client_secrets.json` in the repository root
    directory. If you store it somewhere else, you can set the
    `GOOGLE_CLIENT_SECRETS` environment variable to the appropriate path when
    running the tool.
* Enable the [Youtube Data API](https://console.developers.google.com/apis/api/youtube.googleapis.com)
* Create an OAuth consent screen in the console. You'll need to make the app
    public and enable the YouTube scope. You may submit the app for verification if
    you wish, though that's not required.

## Usage

```sh
make && bin/gopro-uploader --dir $MY_GOPRO_DIR --prefix "MyTrip 2020" --playlist_id $MY_YOUTUBE_PLAYLIST --dry_run
```

> Note: On first use, you may be asked to grant consent for the app to access
> your Google Account. If your app has not been verified yet, you may need to
> click on `Proceed (Unsafe)` option.

This should work with any directory hierarchy. For example, assuming you have
the following:

```
$MY_GOPRO_DIR/Day 1/Person 1/*.MP4
$MY_GOPRO_DIR/Day 1/Person 2/Skiing/*.MP4
$MY_GOPRO_DIR/Day 1/Person 2/Snowboarding/*.MP4
```

The above command will upload the following videos to your playlist:

```
[MyTrip 2020] Day 1 # Person 1
[MyTrip 2020] Day 1 # Person 2 # Skiing
[MyTrip 2020] Day 1 # Person 2 # Snowboarding
```

When you're happy with what the tool will do, run the command without the
`--dry_run` flag.

## Limitations

* The tool uses [ffmpeg concat demuxer](https://ffmpeg.org/ffmpeg-formats.html#concat)
    which allows quickly joining large video files, without reencoding them. If the
    GoPro chapters do not have the same settings (e.g. different resolution, codec),
    the chapters may be grouped into several parts, as opposed to a single video.

* The tool uses Youtube Data API, whose [default quota](https://developers.google.com/youtube/v3/getting-started#quota)
    allows uploading a limited number of videos per day (currently 6). The tool will
    retry API calls when running out of quota, but if you have a large library to
    upload, this will take a while.

* The tool renders and uploads one video at a time. The video is created in a
    temporary directory, so ensure you have enough disk space.

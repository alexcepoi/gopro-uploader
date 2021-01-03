# gopro-uploader

Command line tool which takes a directory hierarchy of GoPro chapter files,
groups and merges them into chaptered videos.

## Requirements

You'll need to have the following installed:

- golang
- ffmpeg


Installation instructions depend on platform. If you use [homebrew](https://brew.sh/),
that's:

```sh
brew install go ffmpeg
```

## Usage

```sh
make && bin/gopro-uploader \
  --input_dir $MY_GOPRO_DIR \
  --output_dir $MY_OUTPUT_DIR \
  --prefix "MyTrip 2020" \
  --dry_run
```

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

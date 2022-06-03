# Bluebot

A Go version of my original Discord bot in Node.js

## Commands

All commands start with the `%` symbol

Currently can just play music from YoutTube links via the `%yt` command:

- `%yt play <URL>` Add a video or playlist to the queue and start playing
- `%yt queue <URL>` Add to the queue once already playing
- `%yt next` Skip forward to the next track
- `%yt pause` Pause the music
- `%yt resume` Resume the music
- `%yt stop` Stop playing and cancel the whole queue
- `%yt list` Show the current queue

## Installation
Can be installed as a systemd service to any remote Linux target that has Go installed (and possibly a couple C libraries from Go library dependencies) with the `scripts/deploy.sh` script.
Just ensure you have a directory called `/opt/bluebot` that you have permission to scp files into. 

Pass the ssh target in as the main argument: `./scripts/deploy.sh joe@myserver`

You will need to add your own discord token to the file `/etc/bluebot/token.txt` after deploying (service start will fail, restart after adding token)

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
Can be installed as a Linux systemd service to the host system or a remote target. Also can be installed locally within the repo folder for testing.
The system you install to must have Go installed as well as `libopus-dev`.

### Systemd
Either run `scripts/install.sh` for a local install or `scripts/deploy.sh` and pass the ssh target in as the main argument e.g. `./scripts/deploy.sh joe@myserver`.
Ensure you have a directory called `/opt/bluebot` on the target and you have permission to scp files into it if it's remote. 

You will need to add your own discord token to the file `/etc/bluebot/token.txt` and a Google YouTube API key file `/etc/bluebot/google_token.json` after deploying (service start will fail, restart after adding token)

### Local test
Run `scripts/install.sh test` in a shell in the repo folder. The necessary folders will be created and the executable built. Then just run `./run.sh` to start the bot. 

As with the systemd install, you must have the 2 required tokens at `./token/token.txt` and `./token/google_token.json`. 

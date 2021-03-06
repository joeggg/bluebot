# Bluebot

The newer Go version of my original Discord bot in Node.js. Just has a small collection of different commands that have been useful on my Discord server.

## Commands

All commands start with the `%` symbol followed by a keyword: `%<command>`.

### **%yt**

Joins the voice channel you are in and plays audio from YoutTube links or search terms, with different commands starting with `%yt` followed by a second keyword:

Usage:
- `%yt play <URL or search term>` Add a video or playlist to the queue and start playing
- `%yt queue <URL or search term>` Add to the queue once already playing
- `%yt next` Skip forward to the next track
- `%yt pause` Pause the music
- `%yt resume` Resume the music
- `%yt stop` Stop playing and cancel the whole queue
- `%yt list` Show the current queue

### **%civ**

Gives a selection of random Civilizations 5 civs to play for a given set of players. Can restrict to give only certain tiers of civ. Intended as a nicer way of more randomly choosing what to play without having to random in-game. Number of civs given is set in config (default is 3)

Usage:
- `%civ <player1> <player2> ...` Generate a selection of civs for the given player names
- `%civ` Regenerate the set of civs based on the last players given in this text channel. Settings persist for 5 minutes - can be set in config
- `%civ tiers <min/max>-<min/max>` Set the min/max tiers to those given (order doesn't matter). The full range of tiers is 1-8. 

### **%say**

Sends a random phrase as a message from a list provided in the folder `data/phrases/say.json`. See the `Phrases` section below for details of custom phrases.

Usage:
- `%say`

### **%tell**
Joins the voice channel you are in and will say a message you provide with the Google Text-to-Speech API (https://cloud.google.com/text-to-speech). Uses a default preset but others can be added and selected. See the `Voice presets` section for details on adding your own presets.

Usage:
- `%tell <message>`
- `setvoice <preset>`


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

## Phrases

In certain instances, the bot will message the text channel with a random phrase from lists of phrases you have to provide. These lists should go in `data/phrases/<name>.json` and will be copied to the correct location when installed (or left there if test install).

The current lists in use are:
- `say.json` for responses to the `%say` command
- `wrongcommand.json` for responses to writing an incorrect command keyword

If no phrases are provided for one of the given situations, the bot will just response with `Hello!`.

The format for the JSON files is:
```
{
    "data": [
        "phrase 1...",
        "phrase 2...",
        ...
    ]
}
```

## Voice presets

Place a JSON file at `data/voice_presets.json` before installing to load your own presets. At least a default voice is required for the `%tell` command to work. The keys for the preset correspond to the settings seen on the Google Text-to-Speech page linked above.

The format for the JSON file is:
```
{
    "default": {
        "language": "en-AU",
        "name": "en-AU-Wavenet-A",
        "pitch": 1,
        "rate": 1,
        "gender": "MALE"
    },
    <voice name>: {
        ...
}
```

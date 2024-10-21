import { Player } from "./player.js"

function inputUrlOnKeypress(event) {
    if (event.key === "Enter") {
        let input_url = document.getElementById("input_url");

        let url = input_url.value;
        input_url.value = "";
    }
}

function setUrlOnClick() {
    let input_url = document.getElementById("input_url");

    let url = input_url.value;
    input_url.value = "";
}

function shiftSubtitlesBack() {
    if (video.textTracks.length === 0) {
        console.warn("NO SUBTITLE TRACKS")
        return;
    }
    let track = video.textTracks[0];
    for (let i = 0; i < video.textTracks.length; i++) {
        video.textTracks[i].mode = "showing";
    }

    console.info("CUES", track.cues)
    for (let i = 0; i < track.cues.length; i++) {
        let cue = track.cues[i];
        cue.startTime -= 0.5;
        cue.endTime -= 0.5;
    }
}

function shiftSubtitlesForward() {
    if (video.textTracks.length === 0) {
        console.warn("NO SUBTITLE TRACKS")
        return;
    }
    let track = video.textTracks[0];
    for (let i = 0; i < video.textTracks.length; i++) {
        video.textTracks[i].mode = "showing";
    }

    console.info("CUES", track.cues)
    for (let i = 0; i < track.cues.length; i++) {
        let cue = track.cues[i];
        cue.startTime += 0.5;
        cue.endTime += 0.5;
    }
}


function attachHtmlElements() {
    window.setUrlOnClick = setUrlOnClick;
    window.inputUrlOnKeypress = inputUrlOnKeypress;
    window.shiftSubtitlesBack  = shiftSubtitlesBack;
    window.shiftSubtitlesForward  = shiftSubtitlesForward;
}

var video = null;

function main() {
    attachHtmlElements();

    let player = new Player();

    let url = "https://download.blender.org/peach/bigbuckbunny_movies/big_buck_bunny_1080p_h264.mov"
    player.createPlayer(url);

    video = document.getElementById("player");
}

main();

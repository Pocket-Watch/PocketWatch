import { Options, Player } from "./custom_player.js"

var dropdownIsDown = true;
var dropdownContainer = document.getElementById("url_dropdown_container");

var entryInputUrl = document.getElementById("url_input_box");
var entryInputTitle = document.getElementById("url_title_input");
var entryInputReferer = document.getElementById("url_dropdown_referer_input");

function dropdownButtonOnClick(event) {
    let button = event.target;

    if (dropdownIsDown) {
        button.textContent = "▲";
        dropdownContainer.style.display = "";
    } else {
        button.textContent = "▼";
        dropdownContainer.style.display = "none";
    }

    dropdownIsDown = !dropdownIsDown;
}

function clearEntryInputElements() {
    entryInputUrl.value = "";
    entryInputTitle.value = "";
    entryInputReferer.value = "";
}

function resetButtonOnClick(event) {
    clearEntryInputElements();
}

function attachHtmlEvents() {
    window.dropdownButtonOnClick = dropdownButtonOnClick;
    window.resetButtonOnClick = resetButtonOnClick;
    dropdownContainer.style.display = "none";
}

let player = null;

function main() {
    attachHtmlEvents();

    let video0 = document.getElementById("video0");
    console.log(video0);
    player = new Player(video0);

    player.setVolume(0.1);

    let test_sub = "media/Elephants.Dream.2006.vtt";
    player.addSubtitleTrack(test_sub)

    // {
    //     let track = "https://ftp.halifax.rwth-aachen.de/blender/demo/movies/ToS/ToS-4k-1920.mov";
    //     player.setVideoTrack(track);
    //     player.setTitle("Tears of Steel");
    //
    //     let subtitle = "media/Tears.of.Steel.2012.vtt";
    //     player.addSubtitleTrack(subtitle)
    // }

    {
        let track = "https://test-streams.mux.dev/x36xhzz/url_6/193039199_mp4_h264_aac_hq_7.m3u8"
        // let track = "https://download.blender.org/peach/bigbuckbunny_movies/BigBuckBunny_640x360.m4v";
        player.setVideoTrack(track);
        player.setTitle("Big Buck Bunny");
    }

    player.seek(17.0);

    player.onControlsPlay(() => {
        player.setToast("User clicked play.");
    })

    player.onControlsPause(() => {
        player.setToast("User clicked pause.");
    })

    player.onControlsSeeked(function(timestamp) {
        let rounded = Math.round(timestamp * 100) / 100.0;
        player.setToast("User seeked to: " + rounded);
    })

    player.onControlsSeeking(function(timestamp) {
        console.log("User seeking to", timestamp);
    })

    player.onPlaybackError(function(event) {
        player.setToast(event.name + " - " + event.message);
    })
}

main();

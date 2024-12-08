import {Options, Player} from "./custom_player.js"

let player = null;

function setOnClick() {
    let input_url = document.getElementById("input_url");
    player.setVideoTrack(input_url.value);
}

function main() {
    window.setOnClick = setOnClick;

    let video0 = document.getElementById("video0");
    let options = new Options();
    // options.hideNextButton = true;
    // options.hideSubtitlesButton = true;
    // options.hideDownloadButton = true;
    player = new Player(video0, options);

    //let track = "https://ftp.halifax.rwth-aachen.de/blender/demo/movies/ToS/ToS-4k-1920.mov";
    let track = "https://test-streams.mux.dev/x36xhzz/url_6/193039199_mp4_h264_aac_hq_7.m3u8"
    // let track = "https://download.blender.org/peach/bigbuckbunny_movies/BigBuckBunny_640x360.m4v";
    let subtitle1 = "media/Elephants.Dream.2006.vtt";
    let subtitle2 = "media/Tears.of.Steel.2012.vtt";
    player.setVideoTrack(track);
    player.setTitle("Tears of Steel");
    player.addSubtitleTrack(subtitle1)
    player.addSubtitleTrack(subtitle2)
    player.setVolume(0.01)
    player.onControlsPlay(() => {
        player.setToast("User clicked play.");
    })

    player.onControlsPause(() => {
        player.setToast("User clicked pause.");
    })

    player.onControlsSeeked(function (timestamp) {
        player.setToast("User seeked to", timestamp);
    })

    player.onControlsSeeking(function (timestamp) {
        player.setToast("User seeking to", timestamp);
    })

    player.onPlaybackError(function (event) {
        console.log(event.name, "-", event.message);
    })
}

main();

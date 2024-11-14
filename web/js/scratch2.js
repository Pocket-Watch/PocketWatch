import {Options, Player} from "./custom_player.js"

let player = null;

function main() {
    let video0 = document.getElementById("video0");
    console.log(video0);
    let options = new Options();
    player = new Player(video0);

    // let track = "https://ftp.halifax.rwth-aachen.de/blender/demo/movies/ToS/ToS-4k-1920.mov";
    let track = "https://test-streams.mux.dev/x36xhzz/url_6/193039199_mp4_h264_aac_hq_7.m3u8"
    // let track = "https://download.blender.org/peach/bigbuckbunny_movies/BigBuckBunny_640x360.m4v";
    //
    player.setVideoTrack(track);
    player.setTitle("Tears of Steel");

    player.onControlsPlay(() => {
        console.log("User clicked play.");
    })

    player.onControlsPause(() => {
        console.log("User clicked pause.");
    })

    player.onControlsSeeked(function (timestamp) {
        console.log("User seeked to", timestamp);
    })

    player.onControlsSeeking(function (timestamp) {
        console.log("User seeking to", timestamp);
    })

    player.onPlaybackError(function (event) {
        console.log(event.name, "-", event.message);
    })
}

main();

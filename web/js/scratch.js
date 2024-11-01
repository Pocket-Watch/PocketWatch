import {Options, Player} from "./custom_player.js"

function main() {
    let video0 = document.getElementById("video0");
    let options = new Options();
    options.hideNextButton = true;
    options.hideSubtitlesButton = true;
    options.hideDownloadButton = true;
    let player = new Player(video0, options);

    let track = "https://ftp.halifax.rwth-aachen.de/blender/demo/movies/ToS/ToS-4k-1920.mov";
    // let track = "https://download.blender.org/peach/bigbuckbunny_movies/BigBuckBunny_640x360.m4v";
    // let track = "http://localhost:1234/watch/media/INVISIBLE.mp4";
    player.setVideoTrack(track);
    player.setTitle("Tears of Steel");

    player.onControlsPlay(() => {
        console.log("User clicked play.");
    })

    player.onControlsPause(() => {
        console.log("User clicked pause.");
    })

    player.onControlsSeek(function(timestamp) {
        console.log("User seeked to", timestamp);
    })
}

main();

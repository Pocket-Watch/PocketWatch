import { Player } from "./custom_player.js"


function main() {
    let video0 = document.getElementById("video0");
    let video1 = document.getElementById("video1");
    let player1 = new Player(video0);
    let player2 = new Player(video1);

    let track = "https://download.blender.org/peach/bigbuckbunny_movies/BigBuckBunny_640x360.m4v";
    //let track = "media/anime.mp4";
    player1.setVideoTrack(track);
    player2.setVideoTrack(track);

    player1.onControlsPlay(() => {
        console.log("User clicked play.");
    })

    player1.onControlsPause(() => {
        console.log("User clicked pause.");
    })

    player1.onControlsSeek(function(timestamp) {
        console.log("User seeked to", timestamp);
    })
}

main();

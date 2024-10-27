import { Player } from "./custom_player.js"


function main() {
    let video1 = document.getElementById("video0");
    let video2 = document.getElementById("video1");
    let player1 = new Player(video1);
    let player2 = new Player(video2);

    let track = "https://download.blender.org/peach/bigbuckbunny_movies/BigBuckBunny_640x360.m4v";
    //let track = "media/anime.mp4";
    player1.setVideoTrack(track);
    player2.setVideoTrack(track);

    player1.onControlsPlay = () => {
        console.log("user clicked play");
    }

    player1.onControlsPause = () => {
        console.log("user clicked pause");
    }
}

main();

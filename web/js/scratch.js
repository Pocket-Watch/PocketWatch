import { Player } from "./custom_player.js"


function main() {
    let player = new Player();
    player.createPlayer();
    player.setVideoTrack("https://download.blender.org/peach/bigbuckbunny_movies/BigBuckBunny_640x360.m4v");

    player.onControlsPlay = () => {
        console.log("user clicked play");
    }

    player.onControlsPause = () => {
        console.log("user clicked pause");
    }
}

main();

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

function attachHtmlElements() {
    window.setUrlOnClick = setUrlOnClick;
    window.inputUrlOnKeypress = inputUrlOnKeypress;
}

function main() {
    attachHtmlElements();

    let player = new Player();
    let url = "https://download.blender.org/peach/bigbuckbunny_movies/big_buck_bunny_1080p_h264.mov"
    player.createPlayer(url);
}

main();

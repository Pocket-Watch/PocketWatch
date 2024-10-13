export { Player };

class Player {
    constructor() {
        // Div container where either the player or the placeholder resides.
        this.root = document.getElementById("player_container");
        // this.root = document.getElementById("player_root");

        // Corresponds to the actual html player element called </video>. 
        // this.video = null;
    }

    destroyPlayer() {
    }

    createPlayerVideo(url) {
        let video = document.createElement('video');
        this.root.appendChild(video);

        let width = window.innerWidth * 0.95;
        video.width = width;
        video.height = width * 9 / 16;
        video.id = "player";
        video.controls = false;

        let source = document.createElement('source');
        video.appendChild(source);
        source.src = url;
    }

    createPlayerControls() {
        let controls = document.createElement('div');
        controls.className = "player_controls_root";

        this.root.appendChild(controls);
    }

    createPlayer(url) {
        this.createPlayerVideo(url);
        this.createPlayerControls(url);
    }
}

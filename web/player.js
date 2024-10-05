class Player {
    constructor() {
        // Div container where either the player or the placeholder resides.
        this.container = document.getElementById("player_container");

        // Corresponds to the fluid player "class" responsible for managing the fluid player.
        this.fluidPlayer = null;

        // Corresponds to the actual html player element called </video>. The fluid player attaches to it.
        this.htmlPlayer = null;

        // Updates on welcome-message and event-message.
        this.serverPlaying = false;

        // Updates before programmatic play() and in eventOnPlay.
        this.programmaticPlay = false;

        // Updates before programmatic pause() and in eventOnPause.
        this.programmaticPause = false;

        // Updates before programmatic currentTime assignment and in eventOnSeek.
        this.programmaticSeek = false;

        // Updates before sending a sync request and on hasty events.
        this.ignoreNextRequest = false;
    }

    eventOnPlay(_event) { 
        if (this.programmaticPlay) {
            this.programmaticPlay = false;
            return;
        }

        if (this.serverPlaying) {
            console.log("WARNING: User triggered play while the server was playing!");
            return;
        }

        // Perform play request here.
    }

    eventOnSeek(_event) { }
    eventOnPause(_event) { }

    subscribeToPlayerEvents() {
        if (!this.fluidPlayer) {
            console.log("WARNING: Failed to subscribe to player events. The player is null.");
            return;
        }

        this.fluidPlayer.on("play", this.eventOnPlay);
        this.fluidPlayer.on("pause", this.eventOnPause);
        this.fluidPlayer.on("seeked", this.eventOnSeek);
    }

    unsubscribeFromPlayerEvents() { 
        if (!this.fluidPlayer) {
            console.log("WARNING: Failed to unsubscribe from player events. The player is null.");
            return;
        }

        this.fluidPlayer.removeEventListener("play", this.eventOnPlay);
        this.fluidPlayer.removeEventListener("pause", this.eventOnPause);
        this.fluidPlayer.removeEventListener("seeked", this.eventOnSeek);
    }

    isVideoPlaying() { }
    createFluidPlayer() { }
    createPlayerPlaceholder() { }

    play() { 
        if (!this.fluidPlayer) {
            console.log("WARNING: Play was triggered, but the fluid player is was not initialized.")
        }
    }
    pause() { }
    seek() { }
    setUrl(_url) { }
}

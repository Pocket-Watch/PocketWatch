const construction = new Image();

function init() {
    construction.src = "under_construction.png";
    // construction.src = "red_box.png";
    window.requestAnimationFrame(draw);
}

let start = 0
let previous = 0

function rad(angle) {
    return (angle * Math.PI) / 180;
}

let acceleration  = rad(20)
const max_speed   = rad(20)
let current_speed = max_speed
let direction     = -1

function draw(timestamp) {
    const elapsed = timestamp - start;
    const dt = (elapsed - previous) / 1000;
    previous = elapsed;

    const ctx = document.getElementById("construction_canvas").getContext("2d");

    ctx.globalCompositeOperation = "destination-over";
    ctx.clearRect(0, 0, 800, 400); // clear canvas

    current_speed += acceleration * dt;

    if (current_speed < 0) {
        direction = -direction;
        acceleration = -acceleration;
        current_speed = 0;
    } 

    if (current_speed > max_speed) {
        acceleration = -acceleration;
        current_speed = max_speed;
    }

    const time = new Date();
    ctx.translate(construction.width / 2, 65);
    ctx.rotate(dt * current_speed * direction);
    ctx.translate(-construction.width / 2, -65);
    ctx.drawImage(construction, 0, 0);
    window.requestAnimationFrame(draw);
}

init();

var slider = document.getElementById("slider");
var sliderContainer = document.getElementById("sliderContainer");
var sliderValue = document.getElementById("sliderValue");
sliderValue.style.pointerEvents = "none";

var isDragging = false;

console.log("Running")

sliderContainer.addEventListener("mousedown", function (e) {
    console.log("sliderContainer mousedown")
    calculateOffset(e)
    isDragging = true;
})

sliderContainer.addEventListener("touchstart", function (e) {
    console.log("sliderContainer touchstart")
    calculateOffset(e)
    isDragging = true;
})


document.addEventListener("mouseup", function() {
    console.log("document mouseup")
    isDragging = false;
});

sliderContainer.addEventListener("touchend", function (e) {
    console.log("sliderContainer touchend")
    isDragging = false;
})

document.addEventListener("mousemove", function(e) {
    console.log("document mousemove")
    if(!isDragging) {
        return;
    }

    calculateOffset(e)
});

document.addEventListener("touchmove", function(e) {
    console.log("document touchmove")
    if(!isDragging) {
        return;
    }

    calculateOffset(e)
});

function calculateOffset(e) {
    let rect = sliderContainer.getBoundingClientRect();
    let offsetX;

    if (e.touches) {
        if (e.touches.length !== 1) {
            return;
        }
        offsetX = e.touches[0].clientX - rect.left;
    } else {
        offsetX = e.clientX - rect.left;
    }

    // Ensure the slider stays within bounds
    if (offsetX < 0) offsetX = 0;
    if (offsetX > rect.width) offsetX = rect.width;

    slider.style.left = offsetX + 'px';

    // Calculate the slider value as a percentage
    let percentage = Math.round((offsetX / rect.width) * 100);
    sliderValue.innerHTML = percentage + "%";
}

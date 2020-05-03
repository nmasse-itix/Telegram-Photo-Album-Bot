
document.addEventListener('DOMContentLoaded', function(event) {
    var videos = document.getElementsByTagName("video");

    for (var i = 0; i < videos.length; i++) {
        var video = videos[i];
        video.addEventListener("mouseenter", function(event) {   
            event.target.muted = false;
            event.target.play();
        });
        
        video.addEventListener("mouseleave", function(event) {   
            event.target.muted = true;
            event.target.pause();
        });
    }
}, false);

// @license magnet:?xt=urn:btih:1f739d935676111cfff4b4693e3816e664797050&dn=gpl-3.0.txt GPL-v3-or-Later

function handleTab(ev) {
    // change tab key behavior to insert tab instead of change focus
    if(ev.keyCode == 9) {
        ev.preventDefault();

        var val = this.value;
        var start = this.selectionStart;
        var end = this.selectionEnd;

        this.value = val.substring(0, start) + '\t' + val.substring(end);
        this.selectionStart = start + 1;
        this.selectionEnd = end + 1;
    }
}

(function () {
    var elem = document.getElementById("delete");
    if (elem !== undefined) {
        elem.addEventListener("click", function (ev) {
            var xhr = new XMLHttpRequest();
            xhr.open("DELETE", document.location.pathname, true);
            xhr.onreadystatechange = function() {
                if (xhr.readyState == 4 && xhr.status === 200) {
                    elem.innerText = "deleted";
                    document.location.reload();
                }
            };
            xhr.send();
            ev.preventDefault();
        });
    }
})();

// @license-end

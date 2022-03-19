// @license magnet:?xt=urn:btih:1f739d935676111cfff4b4693e3816e664797050&dn=gpl-3.0.txt GPL-v3-or-Later

hljs.addPlugin({
    'after:highlight': function (result) {
        result.value = '<div>' + result.value.replaceAll('\n', '\n</div><div>') + '\n</div>'
        var ncode = document.getElementById("normal-code");
        ncode.className = "linenumbers";
    }
});

hljs.highlightAll();

// @license-end

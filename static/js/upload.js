// @license magnet:?xt=urn:btih:1f739d935676111cfff4b4693e3816e664797050&dn=gpl-3.0.txt GPL-v3-or-Later

Dropzone.options.dropzone = {
    init: function () {
        var dzone = document.getElementById("dzone");
        dzone.style.display = "block";
    },
    addedfile: function (file) {
        var upload = document.createElement("div");
        upload.className = "upload";

        var fileLabel = document.createElement("span");
        fileLabel.innerHTML = file.name;
        file.fileLabel = fileLabel;
        upload.appendChild(fileLabel);

        var fileActions = document.createElement("div");
        fileActions.className = "right";
        file.fileActions = fileActions;
        upload.appendChild(fileActions);

        var cancelAction = document.createElement("span");
        cancelAction.className = "cancel";
        cancelAction.innerHTML = "Cancel";
        cancelAction.addEventListener('click', function (ev) {
            this.removeFile(file);
        }.bind(this));
        file.cancelActionElement = cancelAction;
        fileActions.appendChild(cancelAction);

        var progress = document.createElement("span");
        file.progressElement = progress;
        fileActions.appendChild(progress);

        file.uploadElement = upload;

        document.getElementById("uploads").appendChild(upload);

        for (var i=0; i<this.files.length && this.files.length > 1; i++) {
            var f = this.files[i];
            if (f.status === "queued" || f.status === "added")
                f.fileLabel.parentElement.remove();
            if (f.status !== "uploading") {
                this.removeFile(f);
                i--;
            }
        }
    },
    uploadprogress: function (file, p, bytesSent) {
        p = parseInt(p);
        file.progressElement.innerHTML = p + "%";
        file.uploadElement.setAttribute("style", 'background-image: linear-gradient(to right, var(--block-bg-color) ' + p + '%, var(--bg-color) ' + p + '%)');
    },
    sending: function (file, xhr, formData) {
        formData.append("expires", document.getElementById("expires").value);
    },
    success: function (file, resp) {
        file.fileActions.removeChild(file.progressElement);

        var fileLabelLink = document.createElement("a");
        fileLabelLink.href = resp.url;
        fileLabelLink.target = "_blank";
        fileLabelLink.innerHTML = resp.url;
        file.fileLabel.innerHTML = "";
        file.fileLabelLink = fileLabelLink;
        file.fileLabel.appendChild(fileLabelLink);

        var deleteAction = document.createElement("span");
        deleteAction.innerHTML = "Delete";
        deleteAction.className = "cancel";
        deleteAction.addEventListener('click', function (ev) {
            xhr = new XMLHttpRequest();
            xhr.open("DELETE", resp.url, true);
            xhr.setRequestHeader("Linx-Delete-Key", resp.delete_key);
            xhr.onreadystatechange = function (file) {
                if (xhr.readyState == 4 && xhr.status === 200) {
                    var text = document.createTextNode("Deleted ");
                    file.fileLabel.insertBefore(text, file.fileLabelLink);
                    file.fileLabel.className = "deleted";
                    file.fileActions.removeChild(file.cancelActionElement);
                }
            }.bind(this, file);
            xhr.send();
        });
        file.fileActions.removeChild(file.cancelActionElement);
        file.cancelActionElement = deleteAction;
        file.fileActions.appendChild(deleteAction);
    },
    canceled: function (file) {
        this.options.error(file);
    },
    error: function (file, resp, xhrO) {
        file.fileActions.removeChild(file.cancelActionElement);
        file.fileActions.removeChild(file.progressElement);

        if (file.status === "canceled") {
            file.fileLabel.innerHTML = file.name + ": Canceled ";
        }
        else {
            if (resp.error) {
                file.fileLabel.innerHTML = file.name + ": " + resp.error;
            }
            else if (resp.includes("<html")) {
                file.fileLabel.innerHTML = file.name + ": Server Error";
            }
            else {
                file.fileLabel.innerHTML = file.name + ": " + resp;
            }
        }
        file.fileLabel.className = "error";
    },

    autoProcessQueue: true,
    maxFilesize: Math.round(parseInt(document.getElementById("dropzone").getAttribute("data-maxsize"), 10) / 1024 / 1024),
    previewsContainer: "#uploads",
    parallelUploads: 1,
    headers: { "Accept": "application/json" },
    dictDefaultMessage: "Click or Drop file(s) or Paste image",
    dictFallbackMessage: "",
    maxFiles: 1
};

document.onpaste = function (event) {
    var items = (event.clipboardData || event.originalEvent.clipboardData).items;
    for (index in items) {
        var item = items[index];
        if (item.kind === "file") {
            Dropzone.forElement("#dropzone").addFile(item.getAsFile());
        }
    }
};

document.getElementById("access_key_checkbox").onchange = function (event) {
    if (event.target.checked) {
        document.getElementById("access_key_input").style.display = "inline-block";
        document.getElementById("access_key_text").style.display = "none";
    } else {
        document.getElementById("access_key_input").value = "";
        document.getElementById("access_key_input").style.display = "none";
        document.getElementById("access_key_text").style.display = "inline-block";
    }
};

// @end-license

function processForm(event) {
    data = {
        "name": $('#pipeline-name').val(),
        "uri": $('#pipeline-uri').val(),
    };
    $.ajax({
        type: "POST",
        url: "../api/pipelines",
        data: JSON.stringify(data),
        contentType: "application/json",
        success: function (response) {
            result = $('#post-result');
            result.empty();
            result.attr("class", "alert alert-success");
            result.attr("role", "alert");
            result.append($("<p>").append("OK"));
        },
        error: function(jqXHR, textStatus, errorThrown) {
            console.log(textStatus);
            result = $('#post-result');
            result.empty();
            result.attr("class", "alert alert-danger");
            result.attr("role", "alert");
            result.append($("<p>").append(errorThrown));
        }
    });
    return false;
}

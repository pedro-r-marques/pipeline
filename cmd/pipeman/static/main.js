function pipelineTableViewRow(parent, item) {
    var row = $("<tr>")
    parent.append(row)

    var pipelineName = $("<a>").append(item.name)
    pipelineName.attr("href", "pipeline.html?pipeline="+ item.name)
    row.append($("<td>").append(pipelineName))
    // var schema = ['name']
    // for (var i=0; i < schema.length; i++) {
    //     row.append($("<td>").append(item[schema[i]]))
    // }

    var spec = item['spec']
    var schema = ['Namespace', 'Storage']
    for (var i=0; i < schema.length; i++) {
        row.append($("<td>").append(spec[schema[i]]))
    }
}

function loadPipelineTableView(tableElement) {
    $.ajax({
        type: "get",
        url: "../api/pipelines",
        dataType: "json",
        success: function (response) {
            var tbody = tableElement.find('tbody');
            tbody.empty();
            for (var i=0; i < response.length; i++) {
                pipelineTableViewRow(tbody, response[i])
            }
        },
        error: function(jqXHR, textStatus, errorThrown) {
            console.log(textStatus);
        }
    });
}
function addInstance(event) {
    var pipelineName = getUrlParameter('pipeline');

    data = {
        'action': 'start',
        'id': 0,
    };
    $.ajax({
        type: "put",
        url: "../api/state/" + pipelineName,
        data: JSON.stringify(data),
        contentType: "application/json",
        success: function (response) {
            console.log(response);
        },
        error: function(jqXHR, textStatus, errorThrown) {
            console.log(textStatus);
        }
    });

    location.reload();
    return false;
}

function loadPipelineDesc(element) {
    var pipelineName = getUrlParameter('pipeline');
    element.empty();
    element.append(pipelineName);
}

function taskAddItem(tbody, name, tmpl) {
    var elements = ['Image'];
    var row = $('<tr>');
    tbody.append(row);

    row.append($('<td>').append(name));

    for (var j=0; j < elements.length; j++) {
        row.append($('<td>').append(tmpl[elements[j]]));
    }
}

function loadTaskView(tableElement, taskList) {
    var tbody = tableElement.find('tbody');
    tbody.empty();

    for (var i=0; i < taskList.length; i++) {
        var tmplList = taskList[i].TemplateList;
        if (tmplList) {
            for (var j=0; j< tmplList.length; j++) {
                var name = taskList[i].Name + ":" + j.toString();
                taskAddItem(tbody, name, tmplList[j]);  
            }
        } else {
            var tmpl = taskList[i].Template;
            taskAddItem(tbody, taskList[i].Name, tmpl);
        }
    }
}

function loadScheduleView(scheduleElement, spec) {
    if (!("Schedule") in spec || !spec.Schedule) {
        scheduleElement.hide();
        return
    }
    var sched = spec["Schedule"];

    var tbody = scheduleElement.find('tbody');
    tbody.empty();
    var row = $("<tr>");
    tbody.append(row);
    var elements = ['Min', 'Hour', 'Day', 'Weekday', 'Month'];
    for (var i=0; i < elements.length; i++) {
        var item = elements[i];
        if (item in sched && sched[item]) {
            row.append($("<td>").append(sched[item]));
        } else {
            row.append($("<td>").append("*"));        
        }
    }

    scheduleElement.show();
}

function loadInstanceView(tableElement, instanceList) {
    var pipelineName = getUrlParameter('pipeline');

    var tbody = tableElement.find('tbody');
    tbody.empty();

    if (!instanceList) {
        return
    }
    var elements = ['RunStage'];

    for (var i=0; i < instanceList.length; i++) {
        var row = $('<tr>');
        tbody.append(row);

        var instance = instanceList[i];
        var anchor = $('<a>');
        anchor.append(instance.ID);
        anchor.attr('href', 'instance.html?pipeline=' + pipelineName + '&id=' + instance.ID.toString());
        row.append($('<td>').append(anchor));

        for (var j=0; j < elements.length; j++) {
            row.append($('<td>').append(instance[elements[j]]));
        }
    }
}

function loadPipelineDetailView(configElement, scheduleElement, instanceTableElement, taskTableElement) {
    var pipelineName = getUrlParameter('pipeline');

    $.ajax({
        type: "get",
        url: "../api/pipeline/" + pipelineName,
        dataType: "json",
        success: function (response) {
            console.debug(response);
            configElement.empty();
            configElement.append(response.uri);
            loadTaskView(taskTableElement, response.config.spec.Tasks);
            loadScheduleView(scheduleElement, response.config.spec);
            loadInstanceView(instanceTableElement, response.Instances)   
        },
        error: function(jqXHR, textStatus, errorThrown) {
            console.log(textStatus);
        } 
    });
}

function pipelineConfigReload(event) {
    var pipelineName = getUrlParameter('pipeline'); 

    var data = {
    };
    $.ajax({
        type: "put",
        url: "../api/pipeline/" + pipelineName,
        data: JSON.stringify(data),
        contentType: "application/json",
        success: function (response) {
            console.log(response);
            location.reload();
        },
        error: function(jqXHR, textStatus, errorThrown) {
            console.log(textStatus, errorThrown);
        }
    });
}

function pipelineDelete(event) {
    var pipelineName = getUrlParameter('pipeline');
    var data = {};
    $.ajax({
        type: "delete",
        url: "../api/pipeline/" + pipelineName,
        data: JSON.stringify(data),
        contentType: "application/json",
        success: function (response) {
            window.location.href = "index.html";
        },
        error: function(jqXHR, textStatus, errorThrown) {
            console.log(textStatus, errorThrown);
        } 
    });
}

function pipelineActionEvent(event) {
    console.debug($(document.activeElement).attr('id'));
    var btn = $(document.activeElement).attr('id');
    switch(btn) {
    case "reload":
        pipelineConfigReload(event);
        break;
    case "delete":
        pipelineDelete(event);
        break;
    }
    return false;
}
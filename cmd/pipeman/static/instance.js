function loadInstanceDesc(element) {
    var pipelineName = getUrlParameter('pipeline');
    var instanceID = getUrlParameter('id');
    element.empty();
    element.append(pipelineName + ":" + instanceID);
}

function loadInstanceStatusView(taskStatusElement, instance) {
    taskStatusElement.empty();

    var element = $('<div>');
    element.append('State: ');
    element.append(instance.State);
    element.attr('class', 'col-sm-3');
    taskStatusElement.append(element);
    
    elements = ['Running', 'Success', 'Failed']
    for (var i = 0; i < elements.length; i++) {
        var element = $('<div>');
        element.append(elements[i] + ': ');
        element.append(instance.Current[elements[i]]);
        element.attr('class', 'col-sm-3');
        taskStatusElement.append(element);
    }
}

function loadInstanceJobView(jobStatusTable, instance) {
    var tbody = jobStatusTable.find('tbody');
    tbody.empty();

    if (!'JobsStatus' in instance || instance.JobsStatus == null) {
        return
    }
    for (var i=0; i < instance.JobsStatus.length; i++) {
        var row = $('<tr>');
        tbody.append(row);
        var status = instance.JobsStatus[i];
        row.append($('<td>').append(status.JobName));
        var state;
        if (parseInt(status.active) > 0) {
            state = "Running";
        } else {
            state = status.conditions[0].type;
        }
        row.append($('<td>').append(state));
        row.append($('<td>').append(status.active));
        row.append($('<td>').append(status.succeeded));
        row.append($('<td>').append(status.failed));
    }

}

function loadInstanceView(stageElement, taskNameElement, taskStatusElement, jobStatusTable) {
    var pipelineName = getUrlParameter('pipeline');
    var instanceID = parseInt(getUrlParameter('id'));

    $.ajax({
        type: "get",
        url: "../api/pipeline/" + pipelineName,
        dataType: "json",
        success: function(response) {
            var instance;
            var exists = false;
            for (var i=0; i < response.Instances.length; i++) {
                instance = response.Instances[i];
                if (instance.ID == instanceID) {
                    exists = true;
                    break
                }
            }
            if (!exists) {
                console.log(response.Instances.length);
                return
            }
            stageElement.empty();
            stageElement.append('Stage: ' + instance.Stage.toString());
            taskNameElement.empty();
            taskNameElement.append(response.spec.Tasks[instance.Stage].Name);

            loadInstanceStatusView(taskStatusElement, instance);

            loadInstanceJobView(jobStatusTable, instance);
        },
        error: function(jqXHR, textStatus, errorThrown) {
            console.log(textStatus);
        }
    });
}

function cloneInstance(includeVal, excludeVal) {
    var pipelineName = getUrlParameter('pipeline');
    var instanceID = getUrlParameter('id');

    data = {
        'pipeline': pipelineName,
        'instance': parseInt(instanceID),
        'include': includeVal,
        'exclude': excludeVal,
    }
    $.ajax({
        type: "put",
        url: "../api/clone",
        data: JSON.stringify(data),
        dataType: "json",
        success: function (response) {
            console.debug(response);
        },
        error: function(jqXHR, textStatus, errorThrown) {
            console.log(errorThrown);
        }
    });

    return false;
}

function deleteInstance(element) {
    var pipelineName = getUrlParameter('pipeline');
    var instanceID = getUrlParameter('id');

    data = {
        'instance': parseInt(instanceID)
    };
    $.ajax({
        type: "delete",
        url: "../api/pipeline/" + pipelineName,
        data: JSON.stringify(data),
        dataType: "json",
        success: function (response) {
            console.debug(response);
            return false;
        }
    });
    window.location.href = 'pipeline.html?pipeline=' + pipelineName;
    return false;
}

function loadTaskActionTable(actionTable) {
    var pipelineName = getUrlParameter('pipeline');
    var tbody = actionTable.find('tbody');

    $.ajax({
        type: "get",
        url: "../api/pipeline/" + pipelineName,
        dataType: "json",
        success: function(response) {
            var spec = response.spec;
            for (var i=0; i < spec.Tasks.length; i++) {
                var task = spec.Tasks[i];
                var row = $('<tr>');
                tbody.append(row);
                row.append($('<td>').append(task.Name));
                var button = $('<button>');
                row.append(button)
                button.attr('class', 'btn btn-primary');
                button.attr('taskIndex', i);
                button.append('restart');
            }
        }
    });

    actionTable.on('click', 'button', function(){
        var instanceID = getUrlParameter('id');
        var element = $(document.activeElement);
        data = {
            'action': 'start',
            'id': parseInt(instanceID),
            'stage': parseInt(element.attr('taskIndex')),
        };
        $.ajax({
            type: "put",
            url: "../api/state/" + pipelineName,
            data: JSON.stringify(data),
            contentType: "application/json",
            success: function (response) {
                console.log(response);
                location.reload();
            },
            error: function(jqXHR, textStatus, errorThrown) {
                console.log(textStatus);
            }
        });
     });
}
#!/bin/bash

service_name="NetworkManager-dispatcher"
expected_load_state="loaded"
expected_result="success"
expected_active_state="inactive"

while true; do
    # Get the current state of the service
    load_state=$(systemctl show "$service_name" --property=LoadState | cut -d= -f2)
    run_result=$(systemctl show "$service_name" --property=Result | cut -d= -f2)
    active_state=$(systemctl show "$service_name" --property=ActiveState | cut -d= -f2)

    if [ "$load_state" = "$expected_load_state" ] && [ "$run_result" = "$expected_result" ]  && [ "$active_state" = "$expected_active_state" ]; then
        echo "Service $service_name has finished successfuly"
	break
    else
	echo "Service $service_name has not finished yet, load state $load_state, run_result $run_result active_state $active_state"
	sleep 1
    fi
done

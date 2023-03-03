#!/bin/bash
MASTERS=$(oc get nodes --selector='node-role.kubernetes.io/master' -o=jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')
readonly MASTERS

WORKERS=$(oc get nodes --selector='!node-role.kubernetes.io/master' -o=jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')
readonly WORKERS

create_inventory() {
  echo "[masters]" > inventory_hosts
  while IFS= read -r masternode; do
     echo "$masternode" >> inventory_hosts
  done <<< "$MASTERS"
  echo "[workers]" >> inventory_hosts
  while IFS= read -r workernode; do
      echo "$workernode" >> inventory_hosts
  done <<< "$WORKERS"
  # Add workers variables
  echo "[workers:vars]" >> inventory_hosts
}

create_inventory

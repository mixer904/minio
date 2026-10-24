// Copyright 2026 PGSTY contributors.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// helm-migration-guard compares a rendered legacy MinIO chart with the Silo
// upgrade candidate. Product labels, images, and commands may change; resource
// identity, selectors, PVCs, storage mounts, ports, secrets, and service-account
// references must not.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type resource struct {
	key string
	doc map[string]any
}

func main() {
	if len(os.Args) != 3 {
		fatal(errors.New("usage: helm-migration-guard OLD_RENDER NEW_RENDER"))
	}
	oldResources, err := readResources(os.Args[1])
	if err != nil {
		fatal(err)
	}
	newResources, err := readResources(os.Args[2])
	if err != nil {
		fatal(err)
	}
	if err := compare(oldResources, newResources); err != nil {
		fatal(err)
	}
	fmt.Printf("Silo Helm migration identity is stable across %d rendered resources\n", len(oldResources))
}

func readResources(path string) (map[string]resource, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	resources := make(map[string]resource)
	decoder := yaml.NewDecoder(file)
	for document := 1; ; document++ {
		var doc map[string]any
		err = decoder.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode %s document %d: %w", path, document, err)
		}
		if len(doc) == 0 || text(doc["kind"]) == "" {
			continue
		}
		metadata := object(doc["metadata"])
		key := strings.Join([]string{text(doc["kind"]), text(metadata["namespace"]), text(metadata["name"])}, "/")
		if _, exists := resources[key]; exists {
			return nil, fmt.Errorf("%s contains duplicate resource %s", path, key)
		}
		resources[key] = resource{key: key, doc: doc}
	}
	return resources, nil
}

func compare(oldResources, newResources map[string]resource) error {
	for key := range oldResources {
		if _, ok := newResources[key]; !ok {
			return fmt.Errorf("legacy resource would be removed or renamed: %s", key)
		}
	}
	for key := range newResources {
		if _, ok := oldResources[key]; !ok {
			return fmt.Errorf("upgrade candidate unexpectedly adds a resource: %s", key)
		}
	}

	keys := make([]string, 0, len(oldResources))
	for key := range oldResources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		oldDoc := oldResources[key].doc
		newDoc := newResources[key].doc
		kind := text(oldDoc["kind"])
		switch kind {
		case "Service":
			if err := same(key, "Service selector", at(oldDoc, "spec", "selector"), at(newDoc, "spec", "selector")); err != nil {
				return err
			}
			if err := same(key, "Service ports", at(oldDoc, "spec", "ports"), at(newDoc, "spec", "ports")); err != nil {
				return err
			}
		case "Deployment", "StatefulSet":
			if err := same(key, "workload selector", at(oldDoc, "spec", "selector"), at(newDoc, "spec", "selector")); err != nil {
				return err
			}
		}
		if kind == "StatefulSet" {
			if err := same(key, "StatefulSet serviceName", at(oldDoc, "spec", "serviceName"), at(newDoc, "spec", "serviceName")); err != nil {
				return err
			}
			if err := same(key, "volume claim templates", claimTemplates(oldDoc), claimTemplates(newDoc)); err != nil {
				return err
			}
		}
		if kind == "PersistentVolumeClaim" {
			if err := same(key, "PVC specification", at(oldDoc, "spec"), at(newDoc, "spec")); err != nil {
				return err
			}
		}
		if kind == "Secret" {
			if err := same(key, "Secret keys", secretKeys(oldDoc), secretKeys(newDoc)); err != nil {
				return err
			}
		}
		if kind == "Deployment" || kind == "StatefulSet" || kind == "Job" {
			if err := comparePod(key, kind, oldDoc, newDoc); err != nil {
				return err
			}
		}
	}
	return nil
}

func comparePod(key, kind string, oldDoc, newDoc map[string]any) error {
	oldPod := object(at(oldDoc, "spec", "template", "spec"))
	newPod := object(at(newDoc, "spec", "template", "spec"))
	if err := same(key, "service account", oldPod["serviceAccountName"], newPod["serviceAccountName"]); err != nil {
		return err
	}
	if err := same(key, "referenced volume sources", volumeSources(oldPod), volumeSources(newPod)); err != nil {
		return err
	}

	oldContainers := containers(oldPod)
	newContainers := containers(newPod)
	if err := same(key, "container identities", sortedKeys(oldContainers), sortedKeys(newContainers)); err != nil {
		return err
	}
	for _, name := range sortedKeys(oldContainers) {
		oldContainer := oldContainers[name]
		newContainer := newContainers[name]
		if err := same(key, name+" ports", oldContainer["ports"], newContainer["ports"]); err != nil {
			return err
		}
		if err := same(key, name+" environment", oldContainer["env"], newContainer["env"]); err != nil {
			return err
		}
		if err := same(key, name+" envFrom", oldContainer["envFrom"], newContainer["envFrom"]); err != nil {
			return err
		}
		if err := same(key, name+" storage mounts", normalizedMounts(oldContainer, oldPod), normalizedMounts(newContainer, newPod)); err != nil {
			return err
		}

		image := text(newContainer["image"])
		if strings.HasPrefix(image, "pgsty/minio:") || strings.HasPrefix(image, "docker.io/pgsty/minio:") {
			return fmt.Errorf("%s container %s still uses frozen image %s", key, name, image)
		}
		command := commandText(newContainer)
		if strings.Contains(command, "/usr/bin/minio") {
			return fmt.Errorf("%s container %s still invokes /usr/bin/minio", key, name)
		}
		if (kind == "Deployment" || kind == "StatefulSet") && strings.Contains(image, "pgsty/silo:") {
			if !strings.Contains(command, "silo") || !strings.Contains(command, "server") {
				return fmt.Errorf("%s container %s does not invoke the Silo server: %q", key, name, command)
			}
		}
	}
	return nil
}

func claimTemplates(doc map[string]any) []string {
	var result []string
	for _, raw := range list(at(doc, "spec", "volumeClaimTemplates")) {
		claim := object(raw)
		metadata := object(claim["metadata"])
		result = append(result, text(metadata["name"])+"="+canonical(claim["spec"]))
	}
	sort.Strings(result)
	return result
}

func secretKeys(doc map[string]any) []string {
	var result []string
	for _, section := range []string{"data", "stringData"} {
		for key := range object(doc[section]) {
			result = append(result, section+":"+key)
		}
	}
	sort.Strings(result)
	return result
}

func volumeSources(pod map[string]any) []string {
	var result []string
	for _, raw := range list(pod["volumes"]) {
		volume := cloneObject(object(raw))
		delete(volume, "name")
		result = append(result, canonical(volume))
	}
	sort.Strings(result)
	return result
}

func volumeSourceByName(pod map[string]any) map[string]string {
	result := make(map[string]string)
	for _, raw := range list(pod["volumes"]) {
		volume := cloneObject(object(raw))
		name := text(volume["name"])
		delete(volume, "name")
		result[name] = canonical(volume)
	}
	return result
}

func normalizedMounts(container, pod map[string]any) []string {
	sources := volumeSourceByName(pod)
	var result []string
	for _, raw := range list(container["volumeMounts"]) {
		mount := cloneObject(object(raw))
		name := text(mount["name"])
		delete(mount, "name")
		mount["source"] = sources[name]
		result = append(result, canonical(mount))
	}
	sort.Strings(result)
	return result
}

func containers(pod map[string]any) map[string]map[string]any {
	result := make(map[string]map[string]any)
	for _, section := range []string{"initContainers", "containers"} {
		for _, raw := range list(pod[section]) {
			container := object(raw)
			result[section+":"+text(container["name"])] = container
		}
	}
	return result
}

func commandText(container map[string]any) string {
	var parts []string
	for _, field := range []string{"command", "args"} {
		for _, value := range list(container[field]) {
			parts = append(parts, text(value))
		}
	}
	return strings.Join(parts, " ")
}

func at(root map[string]any, path ...string) any {
	var current any = root
	for _, part := range path {
		current = object(current)[part]
	}
	return current
}

func object(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	result, _ := value.(map[string]any)
	return result
}

func cloneObject(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func list(value any) []any {
	result, _ := value.([]any)
	return result
}

func text(value any) string {
	result, _ := value.(string)
	return result
}

func sortedKeys[T any](values map[string]T) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func same(resourceKey, field string, oldValue, newValue any) error {
	oldCanonical := canonical(oldValue)
	newCanonical := canonical(newValue)
	if oldCanonical != newCanonical {
		return fmt.Errorf("%s changes %s\nold: %s\nnew: %s", resourceKey, field, oldCanonical, newCanonical)
	}
	return nil
}

func canonical(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("<unmarshalable %T: %v>", value, err)
	}
	return string(data)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Silo Helm migration check failed: %v\n", err)
	os.Exit(1)
}

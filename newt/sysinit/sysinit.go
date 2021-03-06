/**
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package sysinit

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	log "github.com/Sirupsen/logrus"

	"mynewt.apache.org/newt/newt/newtutil"
	"mynewt.apache.org/newt/newt/pkg"
	"mynewt.apache.org/newt/util"
)

type initFunc struct {
	stage int
	name  string
	pkg   *pkg.LocalPackage
}

func buildStageMap(pkgs []*pkg.LocalPackage) map[int][]*initFunc {
	sm := map[int][]*initFunc{}

	for _, p := range pkgs {
		for name, stage := range p.Init() {
			initFunc := &initFunc{
				stage: stage,
				name:  name,
				pkg:   p,
			}
			sm[stage] = append(sm[stage], initFunc)
		}
	}

	return sm
}

func writePrototypes(pkgs []*pkg.LocalPackage, w io.Writer) {
	sorted := pkg.SortLclPkgs(pkgs)
	for _, p := range sorted {
		init := p.Init()
		for name, _ := range init {
			fmt.Fprintf(w, "void %s(void);\n", name)
		}
	}
}

func writeStage(stage int, initFuncs []*initFunc, w io.Writer) {
	fmt.Fprintf(w, "    /*** Stage %d */\n", stage)
	for i, initFunc := range initFuncs {
		fmt.Fprintf(w, "    /* %d.%d: %s */\n", stage, i, initFunc.pkg.Name())
		fmt.Fprintf(w, "    %s();\n", initFunc.name)
	}
}

func write(pkgs []*pkg.LocalPackage, isLoader bool,
	w io.Writer) {

	stageMap := buildStageMap(pkgs)

	i := 0
	stages := make([]int, len(stageMap))
	for k, _ := range stageMap {
		stages[i] = k
		i++
	}
	sort.Ints(stages)

	fmt.Fprintf(w, newtutil.GeneratedPreamble())

	if isLoader {
		fmt.Fprintf(w, "#if SPLIT_LOADER\n\n")
	} else {
		fmt.Fprintf(w, "#if !SPLIT_LOADER\n\n")
	}

	writePrototypes(pkgs, w)

	var fnName string
	if isLoader {
		fnName = "sysinit_loader"
	} else {
		fnName = "sysinit_app"
	}

	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "void\n%s(void)\n{\n", fnName)

	for _, s := range stages {
		fmt.Fprintf(w, "\n")
		writeStage(s, stageMap[s], w)
	}

	fmt.Fprintf(w, "}\n\n")
	fmt.Fprintf(w, "#endif\n")
}

func writeRequired(contents []byte, path string) (bool, error) {
	oldSrc, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist; write required.
			return true, nil
		}

		return true, util.NewNewtError(err.Error())
	}

	rc := bytes.Compare(oldSrc, contents)
	return rc != 0, nil
}

func EnsureWritten(pkgs []*pkg.LocalPackage, srcDir string, targetName string,
	isLoader bool) error {

	buf := bytes.Buffer{}
	write(pkgs, isLoader, &buf)

	var path string
	if isLoader {
		path = fmt.Sprintf("%s/%s-sysinit-loader.c", srcDir, targetName)
	} else {
		path = fmt.Sprintf("%s/%s-sysinit-app.c", srcDir, targetName)
	}

	writeReqd, err := writeRequired(buf.Bytes(), path)
	if err != nil {
		return err
	}

	if !writeReqd {
		log.Debugf("sysinit unchanged; not writing src file (%s).", path)
		return nil
	}

	log.Debugf("sysinit changed; writing src file (%s).", path)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return util.NewNewtError(err.Error())
	}

	if err := ioutil.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return util.NewNewtError(err.Error())
	}

	return nil
}

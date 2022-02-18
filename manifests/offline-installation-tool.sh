#!/bin/bash

#!/usr/bin/env bash

# Copyright 2018 The KubeSphere Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
CurrentDIR=$(cd "$(dirname "$0")" || exit;pwd)
ImagesDirDefault=${CurrentDIR}/kubesphere-images
save="false"
registryurl=""
reposUrl=("quay.azk8s.cn" "gcr.azk8s.cn" "docker.elastic.co" "quay.io" "k8s.gcr.io", "docker.io", "nvcr.io")
KubernetesVersionDefault="v1.17.9"
HELM_VERSION="v3.2.1"
CNI_VERSION="v0.8.6"

func() {
    echo "Usage:"
    echo
    echo "  $0 [-l IMAGES-LIST] [-d IMAGES-DIR] [-r PRIVATE-REGISTRY] [-v KUBERNETES-VERSION ]"
    echo
    echo "Description:"
    echo "  -b                     : save kubernetes' binaries."
    echo "  -d IMAGES-DIR          : the dir of files (tar.gz) which generated by \`docker save\`. default: ${ImagesDirDefault}"
    echo "  -l IMAGES-LIST         : text file with list of images."
    echo "  -r PRIVATE-REGISTRY    : target private registry:port."
    echo "  -s                     : save model will be applied. Pull the images in the IMAGES-LIST and save images as a tar.gz file."
    echo "  -v KUBERNETES-VERSION  : download kubernetes' binaries. default: v1.17.9"
    echo "  -h                     : usage message"
    exit
}

while getopts 'bsl:r:d:v:h' OPT; do
    case $OPT in
        b) binary="true";;
        d) ImagesDir="$OPTARG";;
        l) ImagesList="$OPTARG";;
        r) Registry="$OPTARG";;
        v) KubernetesVersion="$OPTARG";;
        s) save="true";;
        h) func;;
        ?) func;;
        *) func;;
    esac
done

if [ -z "${ImagesDir}" ]; then
    ImagesDir=${ImagesDirDefault}
fi

if [ -n "${Registry}" ]; then
   registryurl=${Registry}
fi

if [ -z "${KubernetesVersion}" ]; then
   KubernetesVersion=${KubernetesVersionDefault}
fi

if [ -z "${ARCH}" ]; then
  case "$(uname -m)" in
  x86_64)
    ARCH=amd64
    ;;
  armv8*)
    ARCH=arm64
    ;;
  aarch64*)
    ARCH=arm64
    ;;
  armv*)
    ARCH=armv7
    ;;
  *)
    echo "${ARCH}, isn't supported"
    exit 1
    ;;
  esac
fi

binariesDIR=${CurrentDIR}/kubekey/${KubernetesVersion}/${ARCH}

if [[ ${binary} == "true" ]]; then
  mkdir -p ${binariesDIR}
  if [ -n "${KKZONE}" ] && [ "x${KKZONE}" == "xcn" ]; then
     echo "Download kubeadm ..."
     curl -L -o ${binariesDIR}/kubeadm https://kubernetes-release.pek3b.qingstor.com/release/${KubernetesVersion}/bin/linux/${ARCH}/kubeadm
     echo "Download kubelet ..."
     curl -L -o ${binariesDIR}/kubelet https://kubernetes-release.pek3b.qingstor.com/release/${KubernetesVersion}/bin/linux/${ARCH}/kubelet
     echo "Download kubectl ..."
     curl -L -o ${binariesDIR}/kubectl https://kubernetes-release.pek3b.qingstor.com/release/${KubernetesVersion}/bin/linux/${ARCH}/kubectl
     echo "Download helm ..."
     curl -L -o ${binariesDIR}/helm https://kubernetes-helm.pek3b.qingstor.com/linux-${ARCH}/${HELM_VERSION}/helm
     echo "Download cni plugins ..."
     curl -L -o ${binariesDIR}/cni-plugins-linux-${ARCH}-${CNI_VERSION}.tgz https://containernetworking.pek3b.qingstor.com/plugins/releases/download/${CNI_VERSION}/cni-plugins-linux-${ARCH}-${CNI_VERSION}.tgz
  else
     echo "Download kubeadm ..."
     curl -L -o ${binariesDIR}/kubeadm https://storage.googleapis.com/kubernetes-release/release/${KubernetesVersion}/bin/linux/${ARCH}/kubeadm
     echo "Download kubelet ..."
     curl -L -o ${binariesDIR}/kubelet https://storage.googleapis.com/kubernetes-release/release/${KubernetesVersion}/bin/linux/${ARCH}/kubelet
     echo "Download kubectl ..."
     curl -L -o ${binariesDIR}/kubectl https://storage.googleapis.com/kubernetes-release/release/${KubernetesVersion}/bin/linux/${ARCH}/kubectl
     echo "Download helm ..."
     curl -L -o ${binariesDIR}/helm-${HELM_VERSION}-linux-${ARCH}.tar.gz https://get.helm.sh/helm-${HELM_VERSION}-linux-${ARCH}.tar.gz && cd ${binariesDIR} && tar -zxf helm-${HELM_VERSION}-linux-${ARCH}.tar.gz && mv linux-${ARCH}/helm . && rm -rf *linux-${ARCH}* && cd -
     echo "Download cni plugins ..."
     curl -L -o ${binariesDIR}/cni-plugins-linux-${ARCH}-${CNI_VERSION}.tgz https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-linux-${ARCH}-${CNI_VERSION}.tgz
  fi
fi

if [[ ${save} == "true" ]] && [[ -n "${ImagesList}" ]]; then
    if [ ! -d ${ImagesDir} ]; then
       mkdir -p ${ImagesDir}
    fi
    ImagesListLen=$(cat ${ImagesList} | wc -l)
    name=""
    images=""
    index=0
    for image in $(<${ImagesList}); do
        if [[ ${image} =~ ^\#\#.* ]]; then
           if [[ -n ${images} ]]; then
              echo ""
              echo "Save images: "${name}" to "${ImagesDir}"/"${name}".tar.gz  <<<"
              docker save ${images} | gzip -c > ${ImagesDir}"/"${name}.tar.gz
              echo ""
           fi
           images=""
           name=$(echo "${image}" | sed 's/#//g' | sed -e 's/[[:space:]]//g')
           ((index++))
           continue
        fi

        docker pull "${image}"
        images=${images}" "${image}

        if [[ ${index} -eq ${ImagesListLen}-1 ]]; then
           if [[ -n ${images} ]]; then
              docker save ${images} | gzip -c > ${ImagesDir}"/"${name}.tar.gz
           fi
        fi
        ((index++))
    done
elif [ -n "${ImagesList}" ]; then
    # shellcheck disable=SC2045
    # for image in $(ls ${ImagesDir}/*.tar.gz); do
    #  echo "Load images: "${image}"  <<<"
    #  docker load  < $image
    # done

    if [[ -n ${registryurl} ]]; then
       for image in $(<${ImagesList}); do
          if [[ ${image} =~ ^\#\#.* ]]; then
             continue
          fi
          url=${image%%/*}
          ImageName=${image#*/}
          echo $image

          if echo "${reposUrl[@]}" | grep -w "$url" &>/dev/null; then
            imageurl=$registryurl"/"${image#*/}
          elif [ $url == $registryurl ]; then
              if [[ $ImageName != */* ]]; then
                 imageurl=$registryurl"/library/"$ImageName
              else
                 imageurl=$image
              fi
          elif [ "$(echo $url | grep ':')" != "" ]; then
              imageurl=$registryurl"/library/"$image
          else
              imageurl=$registryurl"/"$image
          fi

          ## push image
          echo $imageurl
          docker tag $image $imageurl
          docker push $imageurl
       done
    fi
fi


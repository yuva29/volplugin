FROM yuva29/glusterfs

COPY bin/apiserver /bin/apiserver
COPY bin/volplugin /bin/volplugin
COPY bin/volcli /bin/volcli
COPY bin/volsupervisor /bin/volsupervisor

ENTRYPOINT []

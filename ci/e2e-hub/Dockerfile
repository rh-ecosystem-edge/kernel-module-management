# Create a kmod image that mocks the load of a kmod.
# The e2e-hub test only checks that the kmod image is successfully pulled by the Spoke; it does not verify that the
# kmod is actually loaded.

ARG DTK_AUTO
FROM $DTK_AUTO

RUN ln -s /usr/bin/true /bin/modprobe

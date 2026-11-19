FROM golang:1.26.10
ARG REV
ENV MOCK_REV=$REV
COPY rollout-fixture.test /rollout-fixture.test
ENTRYPOINT ["/rollout-fixture.test", "-test.run=^TestDockerRolloutFixture$", "-test.v"]

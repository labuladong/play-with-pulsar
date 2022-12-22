package org.example;

import java.util.HashMap;
import java.util.Optional;
import org.apache.pulsar.client.api.PulsarClientException;
import org.apache.pulsar.client.api.Schema;
import org.apache.pulsar.client.impl.schema.generic.GenericJsonRecord;
import org.apache.pulsar.common.functions.ConsumerConfig;
import org.apache.pulsar.common.functions.FunctionConfig;
import org.apache.pulsar.functions.LocalRunner;
import org.apache.pulsar.functions.api.Context;
import org.apache.pulsar.functions.api.Function;


public class ScoreboardFunction implements Function<GenericJsonRecord, Void> {

    @Override
    public Void process(GenericJsonRecord input, Context context) {

        String type = (String) input.getField("type");
        if (type.equals("UserDeadEvent")) {
            String player = (String) input.getField("name");
            String killer = (String) input.getField("comment");
           if (player.equals(killer)) {
               // kill himself
               return null;
           }

            // get the source topic of this message
            Optional<String> inputTopic = context.getCurrentRecord().getTopicName();
            if (inputTopic.isEmpty()) {
                return null;
            }
            // calculate the corresponding topic to send score
            Optional<String> outputTopic = changeEventTopicNameToScoreTopicName(inputTopic.get());
            if (outputTopic.isEmpty()) {
                return null;
            }
            // roomName-playerName as the stateful key  /
            // store the score in stateful function
            String killerKey = parseRoomName(inputTopic.get()).get() + "-" + killer;
            context.incrCounter(killerKey, 1);

            // send the score messages to score topic
            long score = context.getCounter(killerKey);
            try {
                context.newOutputMessage(outputTopic.get(), Schema.STRING)
                        .key(killer)
                        .value(score + "")
                        .send();
            } catch (PulsarClientException e) {
                // todo: ignore error for now
                e.printStackTrace();
            }
        }

        return null;
    }

    private Optional<String> parseRoomName(String eventTopicName) {
        int i = eventTopicName.lastIndexOf("-event-topic");
        if (i < 0) {
            return Optional.empty();
        }
        return Optional.of(eventTopicName.substring(0, i));
    }

    private Optional<String> changeEventTopicNameToScoreTopicName(String eventTopicName) {
        Optional<String> roomName = parseRoomName(eventTopicName);
        if (roomName.isEmpty()) {
            return Optional.empty();
        }
        return Optional.of(roomName.get() + "-score-topic");
    }


    public static void main(String[] args) throws Exception {

        FunctionConfig functionConfig = new FunctionConfig();
        functionConfig.setName("score-board-function");

        String inputTopic = ".*-event-topic";
        // enable regex support to subscribe multiple topics
        HashMap<String, ConsumerConfig> inputSpecs = new HashMap<>();
        ConsumerConfig consumerConfig = ConsumerConfig.builder().isRegexPattern(true).build();
        inputSpecs.put(inputTopic, consumerConfig);
        functionConfig.setInputSpecs(inputSpecs);

        functionConfig.setClassName(ScoreboardFunction.class.getName());
        functionConfig.setRuntime(FunctionConfig.Runtime.JAVA);

        functionConfig.setOutputSchemaType("STRING");
        functionConfig.setProcessingGuarantees(FunctionConfig.ProcessingGuarantees.EFFECTIVELY_ONCE);


        LocalRunner localRunner = LocalRunner.builder()
                .brokerServiceUrl("pulsar://localhost:6650")
                .stateStorageServiceUrl("bk://localhost:4181")
                .functionConfig(functionConfig).build();
        localRunner.start(false);
    }
}

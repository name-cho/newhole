import { configureStore, Middleware } from "@reduxjs/toolkit";
import reduxLogger from "redux-logger";
import reduxWebsocket from "@giantmachines/redux-websocket";
import logsReducer from "../features/logs/logsReducer";
import tunnelsReducer from "../features/tunnels/tunnelsReducer";
import configReducer from "../features/config/configReducer";

const reducers = {
  logs: logsReducer,
  tunnels: tunnelsReducer,
  config: configReducer,
};

const store = configureStore({
  reducer: reducers,
  middleware: (getDefaultMiddleware) =>
    getDefaultMiddleware({
      serializableCheck: false,
      immutableCheck: false,
    }).concat(
      reduxLogger as unknown as Middleware,
      reduxWebsocket({
        serializer: JSON.stringify,
        deserializer: JSON.parse,
      }) as unknown as Middleware
    ),
});

export type RootState = ReturnType<typeof store.getState>;
export type AppDispatch = typeof store.dispatch;

export default store;

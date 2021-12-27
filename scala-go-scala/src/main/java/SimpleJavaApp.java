import com.sun.jna.*;

import java.util.Arrays;
import java.util.List;

public class SimpleJavaApp {
    public interface Awesome extends Library {
        // GoSlice class maps to:
        // C type struct { void *data; GoInt len; GoInt cap; }
        public class GoSlice extends Structure {
            public static class ByValue extends GoSlice implements Structure.ByValue {}
            public Pointer data;
            public long len;
            public long cap;
            protected List getFieldOrder(){
                return Arrays.asList(new String[]{"data","len","cap"});
            }
        }

        // GoString class maps to:
        // C type struct { const char *p; GoInt n; }
        public class GoString extends Structure {
            public static class ByValue extends GoString implements Structure.ByValue {}
            public String p;
            public long n;
            protected List getFieldOrder(){
                return Arrays.asList(new String[]{"p","n"});
            }

        }


            public interface NotificationListener extends Callback {
                void invoke();
            }

            public static class NotificationListenerImpl implements NotificationListener {
                @Override
                public void invoke() {
                    System.out.println("invoked from go! hello from java.");
                }
            }


        public interface NotificationParamListener extends Callback {
            void invoke(NativeLong l);
        }

        public static class NotificationParamListenerImpl implements NotificationParamListener {
            @Override
            public void invoke(NativeLong l) {
                System.out.println("invoked from go! hello from java. param: " + l);
            }
        }

        // Foreign functions
        public long Add(long a, long b);
        public double Cosine(double val);
        public void Sort(GoSlice.ByValue vals);
        public long Log(GoString.ByValue str);
        public void DoNothingWithArg(Object o);
        public void CallMe(NotificationListener callback);
        public void CallMeWithSomething(NotificationParamListener callback, NativeLong l);
    }
}

// this was an attempt to figure out how to add a converter in case we wanted to actually pass
// a real java object through a callback back into java, but it turns out a cookie would be almost
// as good, and less cryptic.
class BrokenParam implements NativeMapped {
    public BrokenParam() {
        System.out.println("i am making a Param");
    }
    @Override
    public Object fromNative(Object nativeValue, FromNativeContext context) {
        return "fromNative";
    }

    @Override
    public Object toNative() {
        return "toNative";
    }

    @Override
    public Class<?> nativeType() {
        return String.class;
    }
}

